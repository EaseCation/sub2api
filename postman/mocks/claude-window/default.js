// Models only the short-message contract used by window startup. Uses synthetic
// credentials and fixtures; real provider acceptance is intentionally unverified.
const http = require('node:http')

http.createServer((req, res) => {
  const reply = (status, body, headers = {}) => {
    res.writeHead(status, { 'Content-Type': 'application/json', ...headers })
    res.end(JSON.stringify(body))
  }
  if (req.method !== 'POST' || req.url.split('?')[0] !== '/v1/messages') {
    return reply(501, { error: 'Endpoint not modeled' })
  }
  const body = req.body
  if (!body || body.stream !== false || body.max_tokens !== 1 ||
      body.messages?.length !== 1 || body.messages[0].content !== '.' ||
      body.messages[0].role !== 'user' || body.tools || body.thinking) {
    return reply(400, { error: 'Expected one short message with max_tokens=1' })
  }
  if (body.model === 'test-rate-limit') {
    return reply(429, { type: 'error', error: { type: 'rate_limit_error', message: 'Test limit' } }, { 'Retry-After': '7200' })
  }
  if (body.model === 'test-malformed') return reply(200, {})
  const reset = Math.floor(Date.now() / 1000) + 18000
  return reply(200, {
    id: 'msg_simulation', type: 'message', role: 'assistant', model: body.model,
    content: [{ type: 'text', text: '.' }], stop_reason: 'max_tokens',
    usage: { input_tokens: 20, output_tokens: 1 },
  }, { 'anthropic-ratelimit-unified-5h-reset': String(reset) })
}).listen(process.env.PORT || 4519)
