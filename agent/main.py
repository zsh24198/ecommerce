from contextlib import asynccontextmanager

from typing import Optional

from fastapi import FastAPI, Header, Response
from pydantic import BaseModel

from clients.go_backend import go_client, GoBackendError
from config import settings
from services.llm_service import llm_service
from services.knowledge_base import knowledge_base
from services.session_store import session_store


@asynccontextmanager
async def lifespan(app: FastAPI):
    """服务启动/关闭时的生命周期钩子。"""
    # 启动：构建知识库索引
    count = knowledge_base.load_and_index()
    print(f"[startup] 知识库索引完成，共 {count} 个片段")
    yield
    # 关闭：释放资源
    await go_client.close()
    await llm_service.close()
    await knowledge_base.close()
    await session_store.close()


app = FastAPI(title="Ecommerce Agent", lifespan=lifespan)


class ChatRequest(BaseModel):
    """用户聊天请求体，类比 Go 的 gin.Request DTO。"""
    message: str

@app.get("/health")
def health():
    return {"code": 0, "msg": "ok", "data": {
        "status": "running",
        "go_backend": settings.go_backend_url,
        "port": settings.app_port,
    }}

@app.get("/test-order")
async def test_order():
    """测试 Go API 连通性：硬编码一个 order_no 试试。"""
    try:
        data = await go_client.query_order(user_id=1, order_no="TEST_ORDER_001")
        return {"code": 0, "msg": "ok", "data": data}
    except GoBackendError as e:
        return {"code": e.code, "msg": e.msg, "data": None}
    except Exception as e:
        return {"code": 9999, "msg": f"Go 后端不可达: {e}", "data": None}


@app.post("/chat")
async def chat(
    req: ChatRequest,
    response: Response,
    x_session_id: Optional[str] = Header(default=None, alias="X-Session-Id"),
    x_user_id: Optional[int] = Header(default=None, alias="X-User-Id"),
):
    """Agent 主入口：多轮对话，session_id 缺失则新建并回传。"""
    try:
        session_id = x_session_id or session_store.new_session_id()
        reply = await llm_service.chat(req.message, session_id, user_id=x_user_id)
        response.headers["X-Session-Id"] = session_id
        return {"code": 0, "msg": "ok", "data": {"reply": reply, "session_id": session_id}}
    except Exception as e:
        return {"code": 9999, "msg": f"Agent 处理失败: {e}", "data": None}