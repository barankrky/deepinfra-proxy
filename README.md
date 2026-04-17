# DeepInfra Proxy

OpenAI-compatible API proxy for DeepInfra models.

## Endpoints

| Endpoint | Description |
|----------|-------------|
| `POST /v1/chat/completions` | Chat completions |
| `POST /v1/completions` | Legacy completions |
| `POST /v1/embeddings` | Text embeddings |
| `POST /v1/images/generations` | Image generation |
| `GET /v1/models` | List available models |

## Deploy

```bash
git clone https://github.com/thebarankarakaya/deepinfra-proxy.git
cd deepinfra-proxy
go build -o deepinfra-proxy .
./deepinfra-proxy
```

Server runs on port 8080 by default.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `API_KEY` | None | Optional API key for authentication |
| `PORT` | 8080 | Server port |

```bash
API_KEY=your-secret PORT=8080 ./deepinfra-proxy
```

## Usage

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "meta-llama/Llama-2-70b-chat-hf", "messages": [{"role": "user", "content": "Hello"}]}'
```

API docs available at `http://localhost:8080/docs`

## License

MIT