"""Agent 评估脚本：RAG 召回率 + Tool Accuracy，生成 Markdown 报告。

用法（在 agent 目录下，确保 AGENT_TOKEN / LLM_API_KEY 等环境变量已配置）：
    python eval_agent.py
"""

import asyncio
import uuid
from datetime import datetime
from pathlib import Path

import services.llm_service as llm_mod
from services.knowledge_base import knowledge_base

RECALL_K = 5
REPORT_PATH = Path(__file__).parent / "eval_report.md"

# ========== 数据集：检索用例（口语化，不抄文档原句） ==========
RAG_CASES = [
    {"query": "满300减50能和优惠券一起用吗", "expected_source": "促销规则.md"},
    {"query": "新人注册有什么优惠", "expected_source": "促销规则.md"},
    {"query": "金卡会员打几折", "expected_source": "促销规则.md"},
    {"query": "满减活动可以叠加吗", "expected_source": "促销规则.md"},
    {"query": "优惠券能叠加使用吗", "expected_source": "促销规则.md"},
    {"query": "秒杀商品能参加满减吗", "expected_source": "促销规则.md"},
    {"query": "会员折扣和满减哪个划算", "expected_source": "促销规则.md"},
    {"query": "买了不想要了怎么退", "expected_source": "退换货政策.md"},
    {"query": "7天无理由退货从哪天算起", "expected_source": "退换货政策.md"},
    {"query": "退货运费谁承担", "expected_source": "退换货政策.md"},
    {"query": "退款多久能到账", "expected_source": "退换货政策.md"},
    {"query": "贴身衣物能无理由退货吗", "expected_source": "退换货政策.md"},
    {"query": "换货可以换颜色吗", "expected_source": "退换货政策.md"},
    {"query": "秒杀商品能退货吗", "expected_source": "退换货政策.md"},
    {"query": "多少钱包邮", "expected_source": "运费规则.md"},
    {"query": "新疆发货要多少运费", "expected_source": "运费规则.md"},
    {"query": "港澳台能配送吗", "expected_source": "运费规则.md"},
    {"query": "包邮是按实付还是原价算", "expected_source": "运费规则.md"},
    {"query": "退货后还要补运费吗", "expected_source": "运费规则.md"},
    {"query": "不满99元运费多少", "expected_source": "运费规则.md"},
]

# ========== 数据集：工具调用用例（含陷阱：假ID / 闲聊） ==========
TOOL_CASES = [
    {"input": "帮我查一下订单 20240101120000 的状态", "expected_tool": "query_order"},
    {"input": "订单 99999999999 到货了吗", "expected_tool": "query_order"},
    {"input": "我的订单发货了没", "expected_tool": None},
    {"input": "这件T恤有哪些尺码", "expected_tool": None},
    {"input": "商品 1001 还有货吗", "expected_tool": "query_product"},
    {"input": "这个商品卖多少钱", "expected_tool": None},
    {"input": "查一下用户 1 的信息", "expected_tool": "query_user"},
    {"input": "用户 2 的账号状态", "expected_tool": "query_user"},
    {"input": "退货运费谁承担", "expected_tool": "query_knowledge"},
    {"input": "满300减50能叠加吗", "expected_tool": "query_knowledge"},
    {"input": "多少钱包邮", "expected_tool": "query_knowledge"},
    {"input": "新人券有效期多久", "expected_tool": "query_knowledge"},
    {"input": "你好", "expected_tool": None},
    {"input": "你是谁", "expected_tool": None},
    {"input": "谢谢你的帮助", "expected_tool": None},
]

class ToolRecorder:
    """劫持 execute_tool：记录 LLM 选了哪些工具，用假结果替代真实调用。"""

    def __init__(self):
        self.calls = []

    async def __call__(self, name: str, arguments: dict) -> str:
        self.calls.append({"name": name, "args": arguments})
        return '{"success": true, "data": {}}'

async def capture_tool_call(user_input: str) -> dict:
    """跑一轮对话，返回本轮调用的工具列表与最终回复（不改动生产行为）。"""
    recorder = ToolRecorder()
    original = llm_mod.execute_tool
    llm_mod.execute_tool = recorder
    try:
        reply = await llm_mod.llm_service.chat(user_input, session_id=uuid.uuid4().hex, user_id=1)
    finally:
        llm_mod.execute_tool = original
    return {"calls": recorder.calls, "reply": reply}

async def eval_rag(top_k: int = RECALL_K, ks: tuple = (1, 3, 5)) -> dict:
    """逐条检索（一次 top-k），按结果前缀计算 Recall@k 曲线。"""
    details = []
    for case in RAG_CASES:
        results = await knowledge_base.search(case["query"], top_k=top_k)
        sources = [r["source"] for r in results]
        hits_at = {k: case["expected_source"] in set(sources[:k]) for k in ks}
        details.append({
            "query": case["query"],
            "expected": case["expected_source"],
            "got": sources,
            "hits_at": hits_at,
        })
    total = len(RAG_CASES)
    recall_at = {k: sum(d["hits_at"][k] for d in details) / total if total else 0.0 for k in ks}
    return {"total": total, "top_k": top_k, "ks": ks, "recall_at": recall_at, "details": details}

async def eval_tools() -> dict:
    """逐条对话，统计工具选择准确率。"""
    correct, details = 0, []
    for case in TOOL_CASES:
        result = await capture_tool_call(case["input"])
        called = [c["name"] for c in result["calls"]]
        expected = case["expected_tool"]
        ok = (len(called) == 0) if expected is None else (expected in called)
        correct += int(ok)
        details.append({"input": case["input"], "expected": expected, "called": called, "reply": result["reply"], "ok": ok})
    total = len(TOOL_CASES)
    return {"total": total, "correct": correct, "accuracy": correct / total if total else 0.0, "details": details}

def build_report(rag: dict, tools: dict) -> str:
    """把两项指标渲染成 Markdown 报告。"""
    t = tools["accuracy"] * 100
    lines = [
        "# Agent 评估报告",
        "",
        f"生成时间：{datetime.now():%Y-%m-%d %H:%M:%S}",
        "",
        "## 总览",
        "",
        "| 指标 | 结果 | 目标 | 判定 |",
        "|------|------|------|------|",
    ]
    for k in rag["ks"]:
        v = rag["recall_at"][k] * 100
        lines.append(f"| RAG Recall@{k} | {v:.1f}% | ≥80% | {'✅' if v >= 80 else '❌'} |")
    lines += [
        f"| Tool Selection Accuracy | {t:.1f}% | ≥90% | {'✅' if t >= 90 else '❌'} |",
        "",
        f"用例规模：检索 {rag['total']} 条，工具 {tools['total']} 条",
        "",
        "## RAG 召回详情",
        "",
        "| # | query | 期望文档 | 实际 top-k 来源 | 命中 |",
        "|---|-------|---------|----------------|------|",
    ]
    for i, d in enumerate(rag["details"], 1):
        mark = "✅" if d["hits_at"][rag["top_k"]] else "❌"
        got = ", ".join(d["got"]) or "(空)"
        lines.append(f"| {i} | {d['query']} | {d['expected']} | {got} | {mark} |")

    lines += ["", "## Tool 调用详情", "", "| # | 输入 | 期望工具 | 实际调用 | 命中 |", "|---|------|---------|---------|------|"]
    for i, d in enumerate(tools["details"], 1):
        mark = "✅" if d["ok"] else "❌"
        exp = d["expected"] or "(不调工具)"
        got = ", ".join(d["called"]) or "(未调用)"
        lines.append(f"| {i} | {d['input']} | {exp} | {got} | {mark} |")

    rag_fail = [d for d in rag["details"] if not d["hits_at"][rag["top_k"]]]
    tool_fail = [d for d in tools["details"] if not d["ok"]]
    lines += ["", "## 失败分析", ""]
    if not rag_fail and not tool_fail:
        lines.append("无失败用例。")
    for d in rag_fail:
        lines.append(f"- [RAG] `{d['query']}` 期望 `{d['expected']}`，实际 `{', '.join(d['got'])}`")
    for d in tool_fail:
        lines.append(f"- [Tool] `{d['input']}` 期望 `{d['expected'] or '(不调工具)'}`，实际 `{', '.join(d['called']) or '(未调用)'}`；回复：{d.get('reply', '')}")

    lines += [
        "",
        "## 局限说明",
        "",
        f"- 知识库仅 3 篇文档，按 source 去重后 top-k 最多只有 3 个不同来源，Recall@5 与 Recall@3 基本等价。",
        "- 工具评估用劫持 + 假结果替代真实后端调用，只衡量“工具选择”，不衡量最终回答质量。",
        "- 部分用例缺少查询必需参数（订单号/商品ID），期望行为是反问澄清（不调工具），属正确而非失败。",
        "",
    ]
    return "\n".join(lines)

async def main():
    print("开始评估（检索 + 工具，约需 1-3 分钟）...")
    rag = await eval_rag()
    tools = await eval_tools()
    report = build_report(rag, tools)
    REPORT_PATH.write_text(report, encoding="utf-8")
    print(report)
    print(f"\n报告已写入: {REPORT_PATH}")

if __name__ == "__main__":
    asyncio.run(main())