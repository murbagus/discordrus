// Package discord provides a Discord webhook hook for the Logrus logging library.
// This package allows you to send log entries to Discord channels via webhooks.
//
// Basic usage:
//
//	logger := logrus.New()
//	hook := discord.NewHook("https://discord.com/api/webhooks/...")
//	logger.AddHook(hook)
//	logger.Error("Something went wrong")
//
// HTTP request logging:
//
//	logger.WithField(discord.RequestFieldKey, discord.LoggerHttpRequestPayload{
//		Request: httpRequest,
//	}).Error("API call failed")
//
// Manual request data:
//
//	logger.WithField(discord.RequestFieldKey, discord.LoggerHttpRequestPayload{
//		Method: "POST",
//		URL:    "https://api.example.com/users",
//		BodyString: `{"name": "John"}`,
//	}).Error("User creation failed")
package discordrus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
)

const (
	// REQUEST_FIELD_KEY is the key used to access HTTP request data in logrus fields
	REQUEST_FIELD_KEY = "request"
)

// LoggerHttpRequestPayload holds HTTP request information for logging
// You can either provide a *http.Request or fill the string fields manually
type LoggerHttpRequestPayload struct {
	// Request is the actual HTTP request (preferred)
	Request *http.Request

	// String fields - used when Request is nil or for manual logging
	Method     string // HTTP method (GET, POST, etc.)
	URL        string // Request URL
	BodyString string // Request body as string
	Headers    string // Request headers as string
}

// Hook represents a Discord webhook hook for Logrus
type Hook struct {
	HookUrl string
	lvl     []logrus.Level
}

// NewHook creates a new Discord webhook hook for Logrus
// You can specify the webhook URL and optional log levels to filter
// If no levels are specified, it defaults to Panic, Fatal, Error, and Warn levels
func NewHook(webhookURL string, levels ...logrus.Level) *Hook {
	lvl := levels
	if len(lvl) == 0 {
		lvl = []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
			logrus.WarnLevel,
		}
	}

	return &Hook{
		HookUrl: webhookURL,
		lvl:     lvl,
	}
}

// Levels returns the log levels that this hook will process
func (h *Hook) Levels() []logrus.Level {
	return h.lvl
}

// Fire is called when a log event occurs
func (h *Hook) Fire(entry *logrus.Entry) error {
	if h.HookUrl == "" {
		return eris.New("Discord webhook url is empty")
	}

	// Buat salinan data dari entry.Data["request"] jika ada
	var dataRequestPayload *LoggerHttpRequestPayload
	if v, k := entry.Data[REQUEST_FIELD_KEY]; k {
		if valReq, ok := v.(LoggerHttpRequestPayload); ok {
			// Jika request tidak nil, kita clone request & body-nya
			// agar jika goroutine ini berjalan setelah request selesai,
			// kita masih bisa mendapatkan data request yang valid
			if valReq.Request != nil {
				dataRequestPayload = &LoggerHttpRequestPayload{
					Request: valReq.Request.Clone(valReq.Request.Context()),
				}

				// Membuat copy body jika tersedia
				var bodyBytes []byte
				if valReq.Request.Body != nil {
					bodyBytes, _ = io.ReadAll(valReq.Request.Body)

					// Kembalikan body ke ReadCloser agar kode berikutnya bisa membacanya
					valReq.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
					dataRequestPayload.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Kembalikan body ke ReadCloser
				}
			} else {
				dataRequestPayload = &LoggerHttpRequestPayload{
					Method:     valReq.Method,
					URL:        valReq.URL,
					BodyString: valReq.BodyString,
					Headers:    valReq.Headers,
				}
			}

		}
	}

	go func(drp *LoggerHttpRequestPayload) {
		errorMessage := ""
		if v, k := entry.Data["error"]; k {
			if errVal, ok := v.(error); ok {
				errorMessage = errVal.Error()
			} else if errVal, ok := v.(string); ok {
				errorMessage = errVal
			}
		}

		embedCollor := 12434877 // Warna default
		switch entry.Level {
		case logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel:
			embedCollor = 16725591
		case logrus.WarnLevel:
			embedCollor = 16760630
		default:
			embedCollor = 12434877
		}

		// Request payload fields
		fields := []map[string]interface{}{}

		// Menambahkan request payload field jika tersedia dalam entry.Data["request"]
		if drp != nil {
			if drp.Request != nil {
				fields = append(fields,
					map[string]any{
						"name":  "Method",
						"value": "```" + drp.Request.Method + " ```",
					},
					map[string]any{
						"name":  "URL",
						"value": "```" + drp.Request.URL.String() + " ```",
					},
				)

				// Menambahkan mody sesuai dengan content-type
				var bodyBytes []byte
				if drp.Request.Body != nil {
					bodyBytes, _ = io.ReadAll(drp.Request.Body)

					// Kembalikan body ke ReadCloser agar kode berikutnya bisa membacanya
					drp.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				}

				contentType := drp.Request.Header.Get("Content-Type")
				switch {
				case strings.Contains(contentType, "application/json"):
					fields = append(fields, map[string]any{
						"name":  "Body",
						"value": "```" + string(bodyBytes) + " ```",
					})

				case strings.Contains(contentType, "multipart/form-data"):
					// Untuk multipart, kita tidak bisa dengan mudah membaca semua bagian file ke string.
					// Lebih baik parse form-nya dan catat hanya field non-file.
					// Batas memori untuk parsing form: sesuaikan sesuai kebutuhan
					const maxMemory = 32 << 20 // 32 MB
					if err := drp.Request.ParseMultipartForm(maxMemory); err != nil && err != http.ErrNotMultipart {
						fields = append(fields, map[string]any{
							"name":  "Body",
							"value": "```" + err.Error() + "```",
						})
					} else {
						formData := make(map[string]any)
						for key, values := range drp.Request.MultipartForm.Value {
							if len(values) > 1 {
								formData[key] = values // Bisa jadi slice of strings
							} else {
								formData[key] = values[0] // Bisa jadi slice of strings
							}
						}
						// Jangan log FileHeader secara langsung karena berisi metadata file
						// Anda bisa menambahkan logic untuk mencatat nama file atau ukuran jika diperlukan
						// Misalnya:
						fileInfo := make(map[string]any)
						for key, files := range drp.Request.MultipartForm.File {
							if len(files) > 1 {
								var fileNames []string
								var fileSize []string
								for _, fileHeader := range files {
									fileNames = append(fileNames, fileHeader.Filename)
									fileSize = append(fileSize, fmt.Sprintf("%.2f KB", float64(fileHeader.Size)/1024))
								}

								fileInfo[key] = map[string]any{
									"nama":   fileNames,
									"ukuran": fileSize,
								}
							} else {
								fileInfo[key] = map[string]any{
									"nama":   files[0].Filename,
									"ukuran": fmt.Sprintf("%.2f KB", float64(files[0].Size)/1024),
								}
							}
						}
						// 1. Gabungkan formData dan fileInfo ke dalam satu map
						combinedData := make(map[string]any)
						if len(formData) > 0 {
							combinedData["form_fields"] = formData
						}
						if len(fileInfo) > 0 {
							combinedData["uploaded_files"] = fileInfo
						}

						// 2. Ubah combinedData menjadi string JSON
						jsonString, err := json.MarshalIndent(combinedData, "", "  ") // Gunakan MarshalIndent untuk output yang rapi
						if err == nil {
							fields = append(fields, map[string]any{
								"name":  "Body",
								"value": "```" + string(jsonString) + "```",
							})
						}
					}

				case strings.Contains(contentType, "application/x-www-form-urlencoded"):
					if len(bodyBytes) > 0 {
						parsedForm, err := url.ParseQuery(string(bodyBytes))
						if err != nil {
							fields = append(fields, map[string]any{
								"name":  "Body",
								"value": "```" + string(bodyBytes) + " ```",
							})
						} else {
							formData := make(map[string]interface{})
							for key, values := range parsedForm {
								formData[key] = values
							}
							jsonString, err := json.MarshalIndent(formData, "", "  ") // Gunakan MarshalIndent untuk output yang rapi
							if err == nil {
								fields = append(fields, map[string]any{
									"name":  "Body",
									"value": "```" + string(jsonString) + "```",
								})
							}
						}
					}

				default:
					// Untuk Content-Type lain, catat body mentah jika tidak terlalu besar
					// Pertimbangkan ukuran maksimum untuk logging raw body
					const maxRawBodyLogSize = 1024 // 1 KB
					if len(bodyBytes) > 0 {
						if len(bodyBytes) <= maxRawBodyLogSize {
							fields = append(fields, map[string]any{
								"name":  "Body",
								"value": "```" + string(bodyBytes) + " ```",
							})
						}
					}
				}
			} else {
				if drp.Method != "" {
					fields = append(fields, map[string]interface{}{
						"name":  "Method",
						"value": "```" + drp.Method + " ```",
					})
				}
				if drp.URL != "" {
					fields = append(fields, map[string]interface{}{
						"name":  "URL",
						"value": "```" + drp.URL + " ```",
					})
				}
				if drp.BodyString != "" {
					fields = append(fields, map[string]interface{}{
						"name":  "Body",
						"value": "```" + drp.BodyString + " ```",
					})
				}
				if drp.Headers != "" {
					fields = append(fields, map[string]interface{}{
						"name":  "Headers",
						"value": "```" + drp.Headers + " ```",
					})
				}
			}
		}

		// Jika entry.Message terlalu panjang, kirim sebagai file attachment (txt)
		const maxMessageLength = 500 // Discord embed description max is 4096, tapi biar aman
		messageToSend := entry.Message
		sendAsFile := len(messageToSend) > maxMessageLength

		embeds := []map[string]any{
			{
				"title":       strings.ToUpper(entry.Level.String()),
				"description": errorMessage,
				"timestamp":   entry.Time.UTC().Format(time.RFC3339),
				"color":       embedCollor,
			},
			{
				"title":  "REQUEST PAYLOAD",
				"fields": fields,
				"color":  embedCollor,
			},
		}

		if !sendAsFile {
			embeds = append(embeds, map[string]any{
				"title":       "MESSAGE",
				"description": "```" + messageToSend + " ```",
				"color":       embedCollor,
			})
		}

		payload, err := json.Marshal(map[string]any{
			"username": "Golang",
			"embeds":   embeds,
		})
		if err != nil {
			fmt.Println("Failed to marshal Discord webhook payload:", err)
			return
		}

		if sendAsFile {
			var b bytes.Buffer
			w := io.MultiWriter(&b)

			// Tulis pesan log ke file txt
			_, _ = w.Write([]byte(messageToSend))

			// Buat multipart writer
			var body bytes.Buffer
			mp := multipart.NewWriter(&body)

			// Tambahkan payload_json field
			part, err := mp.CreateFormField("payload_json")
			if err != nil {
				fmt.Println("Failed to create multipart field:", err)
				return
			}
			_, _ = part.Write(payload)

			// Tambahkan file attachment
			filePart, err := mp.CreateFormFile("files[0]", "log.txt")
			if err != nil {
				fmt.Println("Failed to create multipart file:", err)
				return
			}
			_, _ = filePart.Write(b.Bytes())

			mp.Close()

			request, err := http.NewRequest("POST", h.HookUrl, &body)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			request.Header.Set("Content-Type", mp.FormDataContentType())

			client := &http.Client{}
			respons, err := client.Do(request)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			defer respons.Body.Close()

			if respons.StatusCode >= 300 {
				fmt.Println("Failed to post to Discord webhook")
				return
			}
			return
		} else {
			request, err := http.NewRequest("POST", h.HookUrl, bytes.NewBuffer(payload))
			request.Header.Set("Content-Type", "application/json")
			if err != nil {
				fmt.Println(err.Error())
				return
			}

			client := &http.Client{}
			respons, err := client.Do(request)
			if err != nil {
				fmt.Println(err.Error())
				return
			}
			defer respons.Body.Close()

			if respons.StatusCode >= 300 {
				fmt.Println("Failed to post to Discord webhook")
				return
			}
		}

	}(dataRequestPayload)

	return nil
}
