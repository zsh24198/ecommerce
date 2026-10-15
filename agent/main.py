from fastapi import FastAPI
from pydantic import BaseModel

from clients.go_backend import go_client, GoBackendError
from config import settings
from services.llm_service import llm_service

app = FastAPI(title="Ecommerce Agent")


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
async def chat(req: ChatRequest):
    """Agent 主入口：接收用户自然语言，返回 LLM 生成的回复。"""
    try:
        reply = await llm_service.chat(req.message)
        return {"code": 0, "msg": "ok", "data": {"reply": reply}}
    except Exception as e:
        return {"code": 9999, "msg": f"Agent 处理失败: {e}", "data": None}