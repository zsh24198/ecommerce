#!/bin/bash

# 批量注册 1500 个测试用户，登录并落盘 access_token。

# 用户限流 2/s，1500 token × 2/s = 3000 QPS，覆盖阶段 5 的峰值。
set -e

BASE=${BASE:-http://localhost:8080/api/v1/users}
PASSWORD="test123456"
TOTAL=${TOTAL:-1500}
OUT="loadtest/tokens.json"

> "$OUT"
echo "[" >> "$OUT"

for i in $(seq 1 $TOTAL); do
  PHONE=$(printf "1380000%04d" $i)
  curl -s -X POST "$BASE/register" \
       -H "Content-Type: application/json" \
       -d "{\"phone\":\"$PHONE\",\"password\":\"$PASSWORD\"}" > /dev/null || true
  RESP=$(curl -s -X POST "$BASE/login" \
       -H "Content-Type: application/json" \
       -d "{\"phone\":\"$PHONE\",\"password\":\"$PASSWORD\"}")
  TOKEN=$(echo "$RESP" | jq -r '.data.access_token // empty')
  if [ -n "$TOKEN" ]; then
    echo "  {\"phone\":\"$PHONE\",\"token\":\"$TOKEN\"}," >> "$OUT"
  fi
  [ $((i % 100)) -eq 0 ] && echo "progress: $i/$TOTAL" >&2
done

sed -i '$ s/,$//' "$OUT"
echo "]" >> "$OUT"
echo "done: $OUT ($(jq length "$OUT") tokens)" >&2
