"""Session Store —— 多轮对话上下文存储，Redis 优先，异常降级内存。"""

import json
import logging
import uuid

import redis.asyncio as redis

from config import settings

logger = logging.getLogger(__name__)

class SessionStore:
    """按 session_id 存取消息列表：Redis List 为主，进程内存兜底。"""

    KEY_PREFIX = "agent:session:"

    def __init__(self):
        # decode_responses=True：读写直接是 str，省去手动 decode
        self._redis = redis.from_url(settings.redis_url, decode_responses=True)
        self._memory: dict[str, list] = {}  # Redis 不可用时的降级存储

    async def close(self):
        await self._redis.aclose()

    def new_session_id(self) -> str:
        """生成新的会话标识（UUID4 十六进制，不可预测）。"""
        return uuid.uuid4().hex

    async def load(self, session_id: str) -> list:
        """加载历史消息；Redis 异常时降级读内存。"""
        key = self.KEY_PREFIX + session_id
        try:
            raw = await self._redis.lrange(key, 0, -1)
            return [json.loads(m) for m in raw]
        except Exception as e:
            logger.warning(f"Redis 读失败，降级内存: {e}")
            return list(self._memory.get(session_id, []))

    async def append(self, session_id: str, messages: list):
        """追加消息并裁剪条数、刷新 TTL；Redis 异常时降级写内存。"""
        key = self.KEY_PREFIX + session_id
        try:
            payload = [json.dumps(m, ensure_ascii=False) for m in messages]
            await self._redis.rpush(key, *payload)
            await self._redis.ltrim(key, -settings.session_max_messages, -1)
            await self._redis.expire(key, settings.session_ttl_seconds)
        except Exception as e:
            logger.warning(f"Redis 写失败，降级内存: {e}")
            hist = self._memory.setdefault(session_id, [])
            hist.extend(messages)
            del hist[:-settings.session_max_messages]

# 全局单例
session_store = SessionStore()
