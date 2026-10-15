"""Go 后端 Agent API 客户端 —— 异步 httpx 实现。"""

import httpx

from config import settings

class GoBackendError(Exception):
    """Go 后端返回 code != 0 时抛出的业务异常。"""

    def __init__(self, code: int, msg: str):
        self.code = code
        self.msg = msg
        super().__init__(f"GoBackend error [{code}]: {msg}")

class GoBackendClient:
    """封装 Go 后端 /internal/agent/* 的三个查询接口。"""

    def __init__(self):
        self.base_url = settings.go_backend_url.rstrip("/")
        self.headers = {"X-Agent-Token": settings.agent_token}
        # 创建异步客户端，连接池复用（类似 Go 的 http.Client 保持长连接）
        self.client = httpx.AsyncClient(timeout=httpx.Timeout(10.0))

    async def close(self):
        """服务关闭时释放连接池资源。"""
        await self.client.aclose()

    # --- 三个业务接口 ---

    async def query_order(self, user_id: int, order_no: str) -> dict:
        """POST /internal/agent/orders/query"""
        return await self._post("/internal/agent/orders/query", {
            "user_id": user_id,
            "order_no": order_no,
        })

    async def query_product(self, spu_id: int) -> dict:
        """POST /internal/agent/products/query"""
        return await self._post("/internal/agent/products/query", {
            "spu_id": spu_id,
        })

    async def query_user(self, user_id: int) -> dict:
        """POST /internal/agent/users/query"""
        return await self._post("/internal/agent/users/query", {
            "user_id": user_id,
        })

    # --- 内部通用方法 ---

    async def _post(self, path: str, payload: dict) -> dict:
        """统一的 POST + 错误处理。"""
        url = f"{self.base_url}{path}"
        resp = await self.client.post(url, json=payload, headers=self.headers)
        resp.raise_for_status()  # HTTP 层错误（404/500）自动抛 httpx.HTTPError

        body = resp.json()
        # Go 统一格式：{code, msg, data}
        if body.get("code", 1) != 0:
            raise GoBackendError(body["code"], body.get("msg", "unknown"))
        return body.get("data") or {}

# 全局单例（类似 Go 里 main.go 初始化一个 client 传给 service）
go_client = GoBackendClient()