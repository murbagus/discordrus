# Discordrus

[![Go Reference](https://pkg.go.dev/badge/github.com/murbagus/discordrus.svg)](https://pkg.go.dev/github.com/murbagus/discordrus)
[![Go Report Card](https://goreportcard.com/badge/github.com/murbagus/discordrus)](https://goreportcard.com/report/github.com/murbagus/discordrus)

Discordrus is a [Logrus](https://github.com/sirupsen/logrus) hook package that enables you to send log entries to Discord channels via webhooks. This package is specifically designed for HTTP request logging with asynchronous delivery features that don't block your application.

## 🎯 Background

This package was developed to meet the monitoring and debugging needs of Go applications, particularly for:

- **HTTP Request Monitoring**: Record detailed HTTP request information for failed or error requests
- **Real-time Alerting**: Get instant notifications in Discord when errors occur
- **Production Debugging**: Simplify debugging with comprehensive request details
- **Non-blocking Logging**: Webhook delivery is performed asynchronously without hampering application performance

## ✨ Key Features

- ✅ **Asynchronous Delivery** - Webhooks sent in separate goroutines
- ✅ **HTTP Request Logging** - Support for detailed HTTP request logging (method, URL, body, headers)
- ✅ **Multiple Content Types** - Support for JSON, form-data, multipart, and raw body
- ✅ **File Attachments** - Long log messages sent as file attachments
- ✅ **Customizable Log Levels** - Configure which log levels to send
- ✅ **Rich Discord Embeds** - Clean message formatting with color-coded levels
- ✅ **Error Handling** - Graceful handling for various error scenarios

## 📦 Installation

To use this package in your Go project:

```bash
go get github.com/murbagus/discordrus
```

## 🚀 Quick Start

### 1. Basic Logging Setup

```go
package main

import (
    "github.com/sirupsen/logrus"
    "github.com/murbagus/discordrus"
)

func main() {
    // Initialize logger
    logger := logrus.New()

    // Create Discord webhook hook
    hook := discordrus.NewHook("https://discord.com/api/webhooks/YOUR_WEBHOOK_URL")

    // Add hook to logger
    logger.AddHook(hook)

    // Send log to Discord
    logger.Error("Something went wrong in the application!")
}
```

### 2. Custom Log Levels

```go
// Only send Error and Fatal levels to Discord
hook := discordrus.NewHook(
    "https://discord.com/api/webhooks/YOUR_WEBHOOK_URL",
    logrus.ErrorLevel,
    logrus.FatalLevel,
)
```

## 🔧 HTTP Request Logging

### Logging HTTP Request Objects

This package provides a special field for logging HTTP requests with comprehensive details:

```go
func handleAPICall(w http.ResponseWriter, r *http.Request) {
    // Example HTTP request to be logged
    apiRequest, err := http.NewRequest("POST", "https://api.example.com/users",
        strings.NewReader(`{"name": "John Doe", "email": "john@example.com"}`))
    if err != nil {
        logger.WithField(discordrus.REQUEST_FIELD_KEY, discordrus.LoggerHttpRequestPayload{
            Request: apiRequest,
        }).Error("Failed to create API request")
        return
    }

    apiRequest.Header.Set("Content-Type", "application/json")

    // Execute request
    client := &http.Client{}
    resp, err := client.Do(apiRequest)
    if err != nil {
        logger.WithField(discordrus.REQUEST_FIELD_KEY, discordrus.LoggerHttpRequestPayload{
            Request: apiRequest,
        }).Error("API call failed")
        return
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        logger.WithField(discordrus.REQUEST_FIELD_KEY, discordrus.LoggerHttpRequestPayload{
            Request: apiRequest,
        }).Errorf("API returned error status: %d", resp.StatusCode)
    }
}
```

### Manual Request Data

If you don't have an `*http.Request` object, you can fill in the data manually:

```go
logger.WithField(discordrus.REQUEST_FIELD_KEY, discordrus.LoggerHttpRequestPayload{
    Method:     "POST",
    URL:        "https://api.example.com/users",
    BodyString: `{"name": "John Doe", "email": "john@example.com"}`,
    Headers:    "Content-Type: application/json\nAuthorization: Bearer token123",
}).Error("User creation failed")
```

## 📋 Supported Content Types

This package can handle various HTTP content types:

### 1. JSON Requests

```go
// JSON body will be displayed with proper formatting
request.Header.Set("Content-Type", "application/json")
```

### 2. Form Data

```go
// Form data will be parsed and displayed as key-value pairs
request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
```

### 3. Multipart Forms (File Uploads)

```go
// Multipart forms will display field data and file information
request.Header.Set("Content-Type", "multipart/form-data")
```

### 4. Raw Body

```go
// Other content types will be displayed as raw body (max 1KB)
```

## 🎨 Discord Message Format

Logs will be sent to Discord with structured embed formatting:

### Log Level Colors

- 🔴 **Error/Fatal/Panic**: Red
- 🟡 **Warning**: Yellow
- 🔵 **Info/Debug**: Blue

### Embed Structure

1. **Level & Timestamp**: Shows log level and time
2. **Error Message**: Error details if present
3. **Request Payload**: HTTP request details (method, URL, body, headers)
4. **Log Message**: Main log message (as file if too long)

## 🔧 Advanced Configuration

### Middleware Integration

For automatic logging on all HTTP requests:

```go
func LoggingMiddleware(logger *logrus.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Clone request for logging
            bodyBytes, _ := io.ReadAll(r.Body)
            r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

            // Wrapper to capture response status
            wrapper := &responseWrapper{ResponseWriter: w, statusCode: 200}

            // Process request
            next.ServeHTTP(wrapper, r)

            // Log if error occurred
            if wrapper.statusCode >= 400 {
                // Restore body for logging
                r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

                logger.WithField(discordrus.REQUEST_FIELD_KEY, discordrus.LoggerHttpRequestPayload{
                    Request: r,
                }).Errorf("HTTP %d: %s %s", wrapper.statusCode, r.Method, r.URL.Path)
            }
        })
    }
}

type responseWrapper struct {
    http.ResponseWriter
    statusCode int
}

func (rw *responseWrapper) WriteHeader(code int) {
    rw.statusCode = code
    rw.ResponseWriter.WriteHeader(code)
}
```

### Error Context

Add error context for more detailed information:

```go
logger.WithFields(logrus.Fields{
    discordrus.REQUEST_FIELD_KEY: discordrus.LoggerHttpRequestPayload{
        Request: request,
    },
    "error": err,
    "user_id": userID,
    "operation": "create_user",
}).Error("Database operation failed")
```

## 🧪 Example Project

Here's a complete example of usage in a web application:

```go
package main

import (
    "bytes"
    "encoding/json"
    "io"
    "net/http"
    "os"

    "github.com/sirupsen/logrus"
    "github.com/murbagus/discordrus"
)

type Server struct {
    logger *logrus.Logger
}

func main() {
    // Setup logger
    logger := logrus.New()
    logger.SetLevel(logrus.DebugLevel)

    // Setup Discord hook
    webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")
    if webhookURL != "" {
        hook := discordrus.NewHook(webhookURL)
        logger.AddHook(hook)
    }

    server := &Server{logger: logger}

    // Routes
    http.HandleFunc("/api/users", server.createUser)

    logger.Info("Server starting on :8080")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        logger.Fatal("Server failed to start:", err)
    }
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
    if r.Method != "POST" {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Read request body
    bodyBytes, err := io.ReadAll(r.Body)
    if err != nil {
        s.logger.WithField(discordrus.REQUEST_FIELD_KEY, discordrus.LoggerHttpRequestPayload{
            Request: r,
        }).Error("Failed to read request body")
        http.Error(w, "Bad request", http.StatusBadRequest)
        return
    }

    // Restore body for potential re-reading
    r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

    // Parse JSON
    var user struct {
        Name  string `json:"name"`
        Email string `json:"email"`
    }

    if err := json.Unmarshal(bodyBytes, &user); err != nil {
        s.logger.WithField(discordrus.REQUEST_FIELD_KEY, discordrus.LoggerHttpRequestPayload{
            Request: r,
        }).Error("Invalid JSON payload")
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    // Simulate database operation
    if user.Email == "" {
        s.logger.WithField(discordrus.REQUEST_FIELD_KEY, discordrus.LoggerHttpRequestPayload{
            Request: r,
        }).Error("Email is required")
        http.Error(w, "Email is required", http.StatusBadRequest)
        return
    }

    // Success response
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "status": "success",
        "message": "User created successfully",
    })

    s.logger.WithField("user_email", user.Email).Info("User created successfully")
}
```

## 🛠️ Development

### Prerequisites

- Go 1.19 or higher
- Discord server with configured webhook

### Discord Webhook Setup

1. Open your Discord server
2. Go to Server Settings → Integrations → Webhooks
3. Click "New Webhook"
4. Select target channel and copy webhook URL
5. Use that URL in your code

### Testing

```bash
# Clone repository
git clone https://github.com/murbagus/discordrus.git
cd discordrus

# Install dependencies
go mod tidy

# Set environment variable
export DISCORD_WEBHOOK_URL="https://discord.com/api/webhooks/YOUR_WEBHOOK_URL"

# Run tests
go test -v
```

## 📄 License

This package is released under [MIT License](LICENSE).

## 🤝 Contributing

Contributions are very welcome! Please:

1. Fork this repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 🐛 Issues

If you find bugs or have feature requests, please create a new [issue](https://github.com/murbagus/discordrus/issues).

## 📞 Support

For questions or help:

- Open [GitHub Issues](https://github.com/murbagus/discordrus/issues)
- Email: [refi.bahar@gmail.com]

---

**Happy Logging! 🚀**
