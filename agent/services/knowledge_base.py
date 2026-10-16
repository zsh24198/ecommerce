"""RAG 知识库 —— 文档加载、切分、embedding、向量存储、检索。"""

import logging
from pathlib import Path

import chromadb
from chromadb.config import Settings as ChromaSettings
import httpx

from config import settings

logger = logging.getLogger(__name__)

class KnowledgeBase:
    """封装 RAG 全流程：文档 → 切分 → embedding → ChromaDB → 检索。"""

    def __init__(self):
        # 1. 初始化 ChromaDB 客户端（本地持久化，零部署）
        self.chroma = chromadb.PersistentClient(
            path=settings.vector_db_path,
            settings=ChromaSettings(anonymized_telemetry=False),
        )
        # 2. 获取或创建集合（类比数据库里的 table）
        self.collection = self.chroma.get_or_create_collection(
            name="ecommerce_knowledge",
            metadata={"hnsw:space": "cosine"},  # 余弦相似度衡量向量距离
        )
        # 3. embedding API 复用百炼 OpenAI 兼容端点
        self.embedding_url = f"{settings.llm_base_url.rstrip('/')}/embeddings"
        self.headers = {
            "Authorization": f"Bearer {settings.llm_api_key}",
            "Content-Type": "application/json",
        }
        self.client = httpx.AsyncClient(timeout=30.0)

    async def close(self):
        await self.client.aclose()

    # ========== 构建索引：文档 → 向量库 ==========

    def load_and_index(self) -> int:
        """扫描 knowledge_dir 下所有 .md/.txt，切分后写入向量库。返回写入的片段数。"""
        kb_dir = Path(settings.knowledge_dir)
        if not kb_dir.exists():
            logger.warning(f"知识库目录不存在: {kb_dir}")
            return 0

        docs, metadatas, ids = [], [], []
        for f in sorted(kb_dir.iterdir()):
            if f.suffix not in (".md", ".txt"):
                continue
            text = f.read_text(encoding="utf-8").strip()
            if not text:
                continue
            chunks = self._split_text(text, settings.rag_chunk_size, settings.rag_chunk_overlap)
            for i, chunk in enumerate(chunks):
                docs.append(chunk)
                metadatas.append({"source": f.name, "chunk_index": i})
                ids.append(f"{f.name}#{i}")

        if not docs:
            logger.warning("知识库没有有效文档")
            return 0

        self._index_documents(docs, metadatas, ids)
        logger.info(f"知识库索引完成: {len(docs)} 个片段")
        return len(docs)

    def _split_text(self, text: str, chunk_size: int, overlap: int) -> list[str]:
        """固定长度 + overlap 滑动窗口切分。"""
        chunks = []
        start = 0
        while start < len(text):
            end = start + chunk_size
            chunks.append(text[start:end])
            start = end - overlap
            if start >= len(text):
                break
        return chunks

    def _index_documents(self, docs: list[str], metadatas: list[dict], ids: list[str]):
        """调用 embedding API 批量向量化，写入 ChromaDB。"""
        embeddings = self._embed_batch(docs)
        self.collection.upsert(
            ids=ids,
            embeddings=embeddings,
            documents=docs,
            metadatas=metadatas,
        )

    def _embed_batch(self, texts: list[str]) -> list[list[float]]:
        """批量调用百炼 embedding API（同步 httpx，构建索引用）。"""
        with httpx.Client(timeout=60.0) as sync_client:
            resp = sync_client.post(
                self.embedding_url,
                headers=self.headers,
                json={"model": settings.embedding_model, "input": texts},
            )
            resp.raise_for_status()
            data = resp.json()
        return [item["embedding"] for item in data["data"]]

    # ========== 检索：用户问题 → top-k 相关片段 ==========

    async def search(self, query: str) -> list[dict]:
        """用户问题向量化 → 检索 top-k → 返回片段+来源。"""
        query_vec = await self._embed_one(query)
        results = self.collection.query(
            query_embeddings=[query_vec],
            n_results=settings.rag_top_k,
            include=["documents", "metadatas", "distances"],
        )
        docs = results["documents"][0]
        metas = results["metadatas"][0]
        distances = results["distances"][0]
        return [
            {"text": d, "source": m["source"], "score": round(1 - dist, 4)}
            for d, m, dist in zip(docs, metas, distances)
        ]

    async def _embed_one(self, text: str) -> list[float]:
        """单条文本向量化（异步，检索时用）。"""
        resp = await self.client.post(
            self.embedding_url,
            headers=self.headers,
            json={"model": settings.embedding_model, "input": text},
        )
        resp.raise_for_status()
        return resp.json()["data"][0]["embedding"]

# 全局单例
knowledge_base = KnowledgeBase()