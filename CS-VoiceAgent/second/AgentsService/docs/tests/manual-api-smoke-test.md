#!/usr/bin/env bash
set -euo pipefail

BASE="http://localhost:9001"
WS="ws_test"
HDR=(-H "X-Workspace-Id: ${WS}" -H "Content-Type: application/json")

echo "=== 1) healthz ==="
curl -sS "${BASE}/healthz" | jq .

echo "=== 2) runtime без X-Workspace-Id (должен быть 200) ==="
curl -sS -w "\nHTTP %{http_code}\n" "${BASE}/api/v1/runtime/workspaces/published-agents" | head -c 2000
echo

echo "=== 3) protected без X-Workspace-Id (должен быть 400 missing_workspace_id) ==="
curl -sS -w "\nHTTP %{http_code}\n" "${BASE}/api/v1/agents"
echo

echo "=== 4–5) создать агента, взять agent_id и conversation_flow_id ==="
CREATE_RESP="$(curl -sS -X POST "${BASE}/api/v1/agents" "${HDR[@]}" -d '{}')"
echo "${CREATE_RESP}" | jq .
AGENT_ID="$(echo "${CREATE_RESP}" | jq -r '.data.agent_id')"
FLOW_ID="$(echo "${CREATE_RESP}" | jq -r '.data.response_engine.conversation_flow_id')"
echo "AGENT_ID=${AGENT_ID}"
echo "FLOW_ID=${FLOW_ID}"

echo "=== 6) flow v0: published + response_engine ==="
curl -sS "${BASE}/api/v1/agents/${AGENT_ID}/conversation-flows/${FLOW_ID}?version=0" "${HDR[@]}" | jq '.data | {published, response_engine, version}'

echo "=== 7) runtime published-config (ожидается v0 в опубликованном конфиге) ==="
curl -sS "${BASE}/api/v1/runtime/workspaces/${WS}/agents/${AGENT_ID}/published-config" | jq '.data | {agent: .agent.agent_id, cf_version: .conversation_flow.version, pw_version: .published_workflow.version}'

echo "=== 8) создать версию v1 (fromVersion=0) ==="
curl -sS -X POST "${BASE}/api/v1/agents/${AGENT_ID}/conversation-flows/${FLOW_ID}/versions?fromVersion=0" "${HDR[@]}" | jq '.data | {version, conversation_flow_id, published, response_engine}'

echo "=== 9) опубликовать v1 ==="
curl -sS -X POST "${BASE}/api/v1/agents/${AGENT_ID}/conversation-flows/${FLOW_ID}/publish?version=1" "${HDR[@]}" | jq .

echo "=== 10) runtime published-config должен отдавать v1 ==="
curl -sS "${BASE}/api/v1/runtime/workspaces/${WS}/agents/${AGENT_ID}/published-config" | jq '.data | {cf_version: .conversation_flow.version, pw_version: .published_workflow.version}'

echo "=== 11) unpublish (ожидается HTTP 204, тело пустое) ==="
curl -sS -o /dev/null -w "HTTP %{http_code}\n" -X POST "${BASE}/api/v1/agents/${AGENT_ID}/conversation-flows/unpublish" "${HDR[@]}" -d '{}'

echo "=== 12) frontend flow v0 всё ещё открывается (ожидается 200, published=false) ==="
curl -sS "${BASE}/api/v1/agents/${AGENT_ID}/conversation-flows/${FLOW_ID}?version=0" "${HDR[@]}" | jq '.data | {published, response_engine, version}'

echo "=== 13) runtime published-config → 404 published_workflow_not_found ==="
curl -sS -w "\nHTTP %{http_code}\n" "${BASE}/api/v1/runtime/workspaces/${WS}/agents/${AGENT_ID}/published-config" | jq .