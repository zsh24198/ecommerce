"""Tool Registry — 描述 Agent 有哪些工具可调用，以及如何执行。"""

import json

from clients.go_backend import go_client, GoBackendError

# 传给 LLM 的 JSON Schema 数组（OpenAI Function Calling 协议）
# LLM 看到这个就知道：有 3 个工具，每个叫什么、接收什么参数、做什么
TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "query_order",
            "description": "根据订单号查询订单状态和金额。用户需要知道某个订单的详细信息时调用。",
            "parameters": {
                "type": "object",
                "properties": {
                    "user_id": {"type": "integer", "description": "用户 ID，用于校验订单归属。"},
                    "order_no": {"type": "string", "description": "订单号，格式通常为时间戳加随机数。"},
                },
                "required": ["user_id", "order_no"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "query_product",
            "description": "根据商品 SPU ID 查询商品名称、状态和各 SKU 的价格/库存。",
            "parameters": {
                "type": "object",
                "properties": {
                    "spu_id": {"type": "integer", "description": "商品 SPU 主键 ID。"},
                },
                "required": ["spu_id"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "query_user",
            "description": "根据用户 ID 查询用户脱敏信息（不含密码）。",
            "parameters": {
                "type": "object",
                "properties": {
                    "user_id": {"type": "integer", "description": "用户主键 ID。"},
                },
                "required": ["user_id"],
            },
        },
    },
]

async def execute_tool(name: str, arguments: dict) -> str:
    """根据 LLM 返回的工具名 + 参数，实际调用 Go 后端，返回 JSON 字符串。"""
    try:
        if name == "query_order":
            data = await go_client.query_order(**arguments)
        elif name == "query_product":
            data = await go_client.query_product(**arguments)
        elif name == "query_user":
            data = await go_client.query_user(**arguments)
        else:
            return json.dumps({"error": f"未知工具: {name}"}, ensure_ascii=False)
        return json.dumps({"success": True, "data": data}, ensure_ascii=False)
    except GoBackendError as e:
        return json.dumps({"success": False, "code": e.code, "msg": e.msg}, ensure_ascii=False)
    except Exception as e:
        return json.dumps({"success": False, "msg": str(e)}, ensure_ascii=False)