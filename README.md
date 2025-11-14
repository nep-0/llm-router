# LLM Router

A flexible and efficient OpenAI-compatible API router for large language models (LLMs) written in Go. LLM Router intelligently distributes requests across multiple providers and API keys based on usage, enabling load balancing, failover, and cost optimization.

## Features

- **OpenAI-Compatible API** - Drop-in replacement for OpenAI's chat completions endpoint
- **Multi-Provider Support** - Route requests across OpenAI, OpenRouter, and other OpenAI-compatible providers
- **Intelligent Load Balancing** - Automatically selects the API key with lowest usage to distribute load evenly
- **Weighted Model Selection** - Control traffic distribution with configurable weights for cost optimization
- **Model Groups** - Define logical model groups that map to multiple underlying models across different providers
- **Streaming Support** - Full support for streaming chat completions with Server-Sent Events (SSE)
- **API Key Management** - Manage multiple API keys per provider for better rate limiting and redundancy
- **Per-Model Usage Tracking** - Monitors token usage per API key per model for granular routing decisions
- **Compression** - Automatic response compression when client supports it
- **CORS Support** - Built-in CORS handling for browser-based applications
- **Secure Authentication** - Bearer token authentication with constant-time comparison

## Installation

### Prerequisites

- Go 1.25.0 or higher (for building from source)
- Docker (optional, for containerized deployment)

### Using Docker (Recommended)

#### Quick Start with Docker Compose

1. Clone the repository:
```bash
git clone https://github.com/nep-0/llm-router.git
cd llm-router
```

2. Create your configuration file:
```bash
cp config.example.yaml config.yaml
# Edit config.yaml with your settings
```

3. Start the router:
```bash
docker-compose up -d
```

The router will be available at `http://localhost:8080`.

#### Using Docker Directly

Build the image:
```bash
docker build -t llm-router .
```

Run the container:
```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  --name llm-router \
  llm-router
```

### Build from Source

```bash
git clone https://github.com/nep-0/llm-router.git
cd llm-router
go build -o llm-router
```

## Configuration

LLM Router supports both YAML and JSON configuration formats. Copy one of the example configuration files and customize it for your needs:

```bash
cp config.example.yaml config.yaml
```

### Configuration Structure

```yaml
port: 8080
api_key: "your-api-key-here"

groups:
  - name: "gpt-4-turbo"
    models:
      - weight: 1
        provider: "openai"
        name: "gpt-4-turbo-preview"
      - weight: 2
        provider: "openai"
        name: "gpt-4"

  - name: "fast-model"
    models:
      - weight: 1
        provider: "openai"
        name: "gpt-3.5-turbo"
      - weight: 2
        provider: "openrouter"
        name: "openai/gpt-3.5-turbo"

providers:
  - name: "openai"
    base_url: "https://api.openai.com/v1"
    api_keys:
      - "sk-your-openai-key-here"

  - name: "openrouter"
    base_url: "https://openrouter.ai/api/v1"
    api_keys:
      - "sk-your-openrouter-key-here"
```

### Configuration Options

- **port**: HTTP server port (default: 8080)
- **api_key**: Authentication key for accessing the router API
- **groups**: Logical groupings of models
  - **name**: Group identifier (used as the "model" parameter in API requests)
  - **models**: List of models in the group
    - **weight**: Relative weight for load balancing (higher means fewer tokens)
    - **provider**: Provider name (must match a provider definition)
    - **name**: The actual model name to use with the provider
- **providers**: API provider configurations
  - **name**: Provider identifier
  - **base_url**: Provider's base API URL
  - **api_keys**: List of API keys for this provider (enables load balancing)

Note: Weight is inversely proportional to usage; higher weight means the model will be used less frequently. Weight 0 = always use.

## Usage

### Start the Server

#### Using Docker Compose
```bash
docker-compose up -d
```

To view logs:
```bash
docker-compose logs -f
```

To stop:
```bash
docker-compose down
```

#### Using Binary
```bash
./llm-router
```

The server will start on the configured port (default: 8080) and load configuration from `config.yaml`.

### Making API Requests

LLM Router exposes an OpenAI-compatible endpoint at `/v1/chat/completions`.

#### Non-Streaming Request

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key-here" \
  -d '{
    "model": "gpt-4-turbo",
    "messages": [
      {
        "role": "user",
        "content": "Hello, how are you?"
      }
    ]
  }'
```

#### Streaming Request

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key-here" \
  -d '{
    "model": "gpt-4-turbo",
    "messages": [
      {
        "role": "user",
        "content": "Tell me a story"
      }
    ],
    "stream": true
  }'
```

### Using with OpenAI Client Libraries

LLM Router is compatible with official OpenAI client libraries. Simply change the base URL:

#### Python

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-api-key-here",
    base_url="http://localhost:8080/v1"
)

response = client.chat.completions.create(
    model="gpt-4-turbo",  # Use your group name
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
```

## How It Works

1. **Request Reception**: The router receives a request for a model group (e.g., "gpt-4-turbo")
2. **Model Selection**: Based on the group configuration, the router identifies available models across different providers
3. **Load Balancing**: The router selects the API key with the lowest current usage from the available providers
4. **Request Forwarding**: The request is forwarded to the selected provider with the appropriate model name
5. **Usage Tracking**: Token usage is tracked and attributed to the specific API key used
6. **Response Return**: The provider's response is returned to the client

## Project Structure

```
llm-router/
├── app/                  # Application logic and request handling       
├── client/               # Provider client wrappers and usage tracking       
├── config/               # Configuration loading and parsing
├── server/               # HTTP server and request routing
├── utils/                # Utility functions for logging and request handling       
├── main.go               # Application entry point
├── go.mod                # Go module dependencies
├── config.yaml           # Configuration file (not included, use example)
├── Dockerfile            # Dockerfile for containerization
└── docker-compose.yml    # Docker Compose configuration file
```

## Dependencies

- [go-openai](https://github.com/sashabaranov/go-openai) - OpenAI API client library
- [viper](https://github.com/spf13/viper) - Configuration management
- [brotli](https://github.com/andybalholm/brotli) - Brotli compression

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Credits

Portions of this project are based on code from [kcolemangt/llm-router](https://github.com/kcolemangt/llm-router).

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Roadmap

- [ ] Health checks and automatic provider failover
- [ ] Web UI for monitoring and configuration

## Support

For issues, questions, or contributions, please open an issue on the project repository.
