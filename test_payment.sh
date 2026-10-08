#!/bin/bash
BASE="http://localhost:8080"

echo "=== 1. 注册新用户 ==="
curl -s -X POST $BASE/api/v1/users/register -H "Content-Type: application/json" -d '{"phone":"13900002222","password":"123456"}'
echo

echo "=== 2. 登录 ==="
LOGIN_RESP=$(curl -s -X POST $BASE/api/v1/users/login -H "Content-Type: application/json" -d '{"phone":"13900002222","password":"123456"}')
echo "$LOGIN_RESP"
TOKEN=$(echo "$LOGIN_RESP" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)
echo "TOKEN 长度: ${#TOKEN}"

echo "=== 3. 创建商品 ==="
PROD_RESP=$(curl -s -X POST $BASE/api/v1/products -H "Content-Type: application/json" -d '{"name":"测试商品","main_image":"https://example.com/img.jpg","skus":[{"name":"默认规格","specs":{"颜色":"红色"},"price_cents":9900,"stock":100}]}')
echo "$PROD_RESP"
SKU_ID=$(echo "$PROD_RESP" | grep -o '"id":[0-9]*' | head -2 | tail -1 | cut -d':' -f2)
echo "SKU_ID: $SKU_ID"

echo "=== 4. 创建订单 ==="
ORDER_RESP=$(curl -s -X POST $BASE/api/v1/orders -H "Content-Type: application/json" -H "Authorization: Bearer $TOKEN" -H "Idempotency-Key: $(cat /proc/sys/kernel/random/uuid)" -d "{\"items\":[{\"sku_id\":$SKU_ID,\"quantity\":1}],\"receiver_name\":\"张三\",\"receiver_phone\":\"13900002222\",\"receiver_addr\":\"测试地址\"}")
echo "$ORDER_RESP"
ORDER_NO=$(echo "$ORDER_RESP" | grep -o '"order_no":"[^"]*"' | cut -d'"' -f4)
echo "ORDER_NO: $ORDER_NO"

echo "=== 5. 创建支付单 ==="
PAY_RESP=$(curl -s -X POST $BASE/api/v1/payments -H "Content-Type: application/json" -H "Authorization: Bearer $TOKEN" -d "{\"order_no\":\"$ORDER_NO\",\"channel\":1}")
echo "$PAY_RESP"
PAYMENT_NO=$(echo "$PAY_RESP" | grep -o '"payment_no":"[^"]*"' | cut -d'"' -f4)
echo "PAYMENT_NO: $PAYMENT_NO"

if [ -z "$PAYMENT_NO" ] || [ -z "$ORDER_NO" ]; then
  echo "❌ 关键变量为空，退出"
  exit 1
fi

echo "=== 6. 计算签名 ==="
AMOUNT="9900"; CHANNEL="1"; TRANSACTION_ID="420000123420261004$(date +%s)"
STATUS="success"; PAID_AT="2026-10-04 12:00:01"; SECRET="dev-payment-sign-secret-change-me"
SIGN=$(printf 'amount=%s&channel=%s&order_no=%s&paid_at=%s&payment_no=%s&status=%s&transaction_id=%s&key=%s' "$AMOUNT" "$CHANNEL" "$ORDER_NO" "$PAID_AT" "$PAYMENT_NO" "$STATUS" "$TRANSACTION_ID" "$SECRET" | openssl dgst -sha256 -hmac "$SECRET" | awk '{print toupper($2)}')
echo "SIGN: $SIGN"

echo "=== 7. 第一次回调（预期 SUCCESS）==="
curl -s -X POST $BASE/api/v1/payments/callback -H "Content-Type: application/json" -d "{\"payment_no\":\"$PAYMENT_NO\",\"order_no\":\"$ORDER_NO\",\"amount\":\"$AMOUNT\",\"channel\":$CHANNEL,\"transaction_id\":\"$TRANSACTION_ID\",\"status\":\"$STATUS\",\"paid_at\":\"$PAID_AT\",\"sign\":\"$SIGN\"}"
echo

echo "=== 8. 第二次回调（幂等，预期 SUCCESS）==="
curl -s -X POST $BASE/api/v1/payments/callback -H "Content-Type: application/json" -d "{\"payment_no\":\"$PAYMENT_NO\",\"order_no\":\"$ORDER_NO\",\"amount\":\"$AMOUNT\",\"channel\":$CHANNEL,\"transaction_id\":\"$TRANSACTION_ID\",\"status\":\"$STATUS\",\"paid_at\":\"$PAID_AT\",\"sign\":\"$SIGN\"}"
echo

echo "=== 9. 错误签名回调（预期 FAIL）==="
curl -s -X POST $BASE/api/v1/payments/callback -H "Content-Type: application/json" -d "{\"payment_no\":\"$PAYMENT_NO\",\"order_no\":\"$ORDER_NO\",\"amount\":\"$AMOUNT\",\"channel\":$CHANNEL,\"transaction_id\":\"$TRANSACTION_ID\",\"status\":\"$STATUS\",\"paid_at\":\"$PAID_AT\",\"sign\":\"WRONG_SIGN\"}"
echo

echo "=== 10. MySQL 容器名 ==="

docker ps --format '{{.Names}}' | grep -i mysql