from pydantic_settings import BaseSettings, SettingsConfigDict

class Settings(BaseSettings):
    """Agent 服务配置，全部从环境变量读取。"""

    # Go 后端 —— Python Agent 要调的目标
    go_backend_url: str = "http://localhost:8080"
    agent_token: str  # 必填，无默认值（敏感配置）

    # 服务自身
    app_host: str = "0.0.0.0"
    app_port: int = 9000

    # LLM —— 阿里云百炼 OpenAI 兼容模式
    llm_base_url: str = "https://dashscope.aliyuncs.com/compatible-mode/v1"
    llm_api_key: str  # 必填，从环境变量注入，禁止硬编码
    llm_model: str = "qwen-plus"

    # 告诉 pydantic-settings 去哪找配置
    model_config = SettingsConfigDict(
        env_file=".env",              # 可选：本地开发可以用 .env 文件
        env_file_encoding="utf-8",
        case_sensitive=False,         # 环境变量名不区分大小写
    )

# 全局单例 —— import 时自动从环境变量加载
settings = Settings()