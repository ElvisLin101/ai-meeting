#!/bin/bash
# 面试压测脚本：批量调用出题+答题，产生指标数据
# 用法: bash scripts/stress_test.sh <轮数>

ROUNDS=${1:-20}
BASE="http://localhost:8080/api/xunzhi/v1"

echo "=== 压测开始: $ROUNDS 轮面试 ==="

# 登录
TOKEN=$(curl -s -X POST "$BASE/users/login" -H "Content-Type: application/json" -d '{"username":"testuser","password":"123456"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
if [ -z "$TOKEN" ]; then
  echo "登录失败"
  exit 1
fi
echo "登录成功"

RESUME="张三，3年Go后端开发经验，熟悉Gin/GORM/MySQL/Redis，做过电商订单系统和支付系统，熟悉微服务架构和Kafka消息队列。了解Docker和Kubernetes基本使用。"

for i in $(seq 1 $ROUNDS); do
  echo "--- 第 $i 轮 ---"

  # 创建会话
  SESSION=$(curl -s -X POST "$BASE/interview/sessions" -H "Authorization: Bearer $TOKEN" | grep -o '"session_id":"[^"]*"' | cut -d'"' -f4)
  if [ -z "$SESSION" ]; then
    echo "  创建会话失败"
    continue
  fi

  # 出题
  Q1=$(curl -s -X POST "$BASE/interview/sessions/$SESSION/interview-questions" \
    -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
    -d "{\"resume_content\":\"$RESUME\"}" 2>/dev/null)
  QNUM=$(echo "$Q1" | grep -o '"question_number":"[^"]*"' | cut -d'"' -f4)
  if [ -z "$QNUM" ]; then
    echo "  出题失败: $Q1"
    continue
  fi
  echo "  出题成功: Q$QNUM"

  # 答第一题
  ANS1=$(curl -s -X POST "$BASE/interview/sessions/$SESSION/interview/answer-json" \
    -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
    -d "{\"question_number\":\"$QNUM\",\"answer_content\":\"Gin路由分组通过RouterGroup实现，中间件在请求前后执行。比如电商系统里可以把/api/v1/order归到订单组加鉴权中间件。\",\"request_id\":\"req-$i-1\"}" 2>/dev/null)
  SCORE1=$(echo "$ANS1" | grep -o '"score":[0-9]*' | cut -d':' -f2)
  NEXT_Q=$(echo "$ANS1" | grep -o '"next_question_number":"[^"]*"' | cut -d'"' -f4)
  echo "  答题1: score=$SCORE1, next=$NEXT_Q"

  # 如果有追问，答追问
  if [ -n "$NEXT_Q" ] && [ "$NEXT_Q" != "" ]; then
    sleep 1
    ANS2=$(curl -s -X POST "$BASE/interview/sessions/$SESSION/interview/answer-json" \
      -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
      -d "{\"question_number\":\"$NEXT_Q\",\"answer_content\":\"func AuthMiddleware() gin.HandlerFunc { return func(c *gin.Context) { token := c.GetHeader(\\\"Authorization\\\") if token == \\\"\\\" { c.AbortWithStatusJSON(401, gin.H{\\\"error\\\":\\\"unauthorized\\\"}) return } c.Next() } }\",\"request_id\":\"req-$i-2\"}" 2>/dev/null)
    SCORE2=$(echo "$ANS2" | grep -o '"score":[0-9]*' | cut -d':' -f2)
    echo "  答题2(追问): score=$SCORE2"
  fi

  sleep 0.5
done

echo "=== 压测完成 ==="
