"""LLM Service —— 裸调百炼 OpenAI 兼容 API，实现 ReAct 循环。"""

import json
import logging

import httpx

from config import settings
from services.tools import TOOLS, execute_tool

logger = logging.getLogger(__name__)

class LLMService:
    """封装百炼 API 调用 + Agent ReAct 循环。"""

    SYSTEM_PROMPT = (
        "你是电商智能客服助手，可以帮用户查询订单、商品、用户信息，以及解答业务规则问题。\n"
        "遇到以下两类问题时必须调用工具，不得凭记忆编造：\n"
        "1. 查询动态数据（订单状态、商品库存、用户信息）→ 调用 query_order / query_product / query_user\n"
        "2. 业务规则问题（促销活动、退换货政策、运费规则、商品使用说明）→ 调用 query_knowledge\n"
        "拿到工具结果后，用自然语言回复用户。如果知识库没有相关内容，如实告知用户。"
    )

    def __init__(self):
        # 百炼 OpenAI 兼容模式的 base_url 已经带了 /v1 后缀
        self.base_url = settings.llm_base_url.rstrip("/")
        self.headers = {
            "Authorization": f"Bearer {settings.llm_api_key}",
            "Content-Type": "application/json",
        }
        self.client = httpx.AsyncClient(timeout=60.0)

    async def close(self):
        await self.client.aclose()

    async def chat(self, user_input: str) -> str:
        """Agent 主循环：用户输入 → 调工具（如有）→ 生成回复。"""
        messages = [
            {"role": "system", "content": self.SYSTEM_PROMPT},
            {"role": "user", "content": user_input},
        ]

        # 最多循环 5 次，防止 LLM 无限调工具
        for _ in range(5):
            response = await self._call_llm(messages, TOOLS)

            # 情况 1：LLM 直接返回文本，没有调工具
            tool_calls = response.get("tool_calls")
            if not tool_calls:
                return response["content"]

            # 情况 2：LLM 要调工具，执行并把结果塞回去
            messages.append({
                "role": "assistant",
                "content": response.get("content", ""),
                "tool_calls": tool_calls,
            })

            for tc in tool_calls:
                fn = tc["function"]
                name = fn["name"]
                args = json.loads(fn["arguments"])  # LLM 返回的 arguments 是字符串

                logger.info(f"LLM 调用工具: {name}({args})")
                result = await execute_tool(name, args)

                messages.append({
                    "role": "tool",
                    "tool_call_id": tc["id"],
                    "content": result,
                })

        # 循环超限兜底
        return "抱歉，我没能在限定步数内完成查询，请稍后重试。"

    async def _call_llm(self, messages: list, tools: list = None) -> dict:
        """裸调百炼 OpenAI 兼容 API，返回 assistant message。"""
        payload = {
            "model": settings.llm_model,
            "messages": messages,
        }
        if tools:
            payload["tools"] = tools

        url = f"{self.base_url}/chat/completions"
        resp = await self.client.post(url, json=payload, headers=self.headers)
        resp.raise_for_status()

        body = resp.json()
        return body["choices"][0]["message"]

# 全局单例
llm_service = LLMService()