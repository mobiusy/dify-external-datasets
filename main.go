package main

import (
	"dify-external-datasets/ragflow"
	"encoding/json"
	"log"
	"net/http"

	"github.com/joho/godotenv"
)

// ErrorResponse 定义错误响应的结构
type ErrorResponse struct {
	ErrorCode int    `json:"error_code"`
	ErrorMsg  string `json:"error_msg"`
}

// ErrorHandler 统一错误处理中间件
func errorHandler(handler func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := handler(w, r); err != nil {
			if customErr, ok := err.(*ragflow.CustomError); ok {
				statusCode := http.StatusInternalServerError
				switch customErr.Code {
				case 400:
					statusCode = http.StatusBadRequest
				case 1001, 1002:
					statusCode = http.StatusForbidden
				case 2001:
					statusCode = http.StatusNotFound
				}
				w.WriteHeader(statusCode)
				json.NewEncoder(w).Encode(ErrorResponse{
					ErrorCode: customErr.Code,
					ErrorMsg:  customErr.Message,
				})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{
				ErrorCode: 500,
				ErrorMsg:  "内部服务器错误",
			})
		}
	}
}

func main() {
	// 加载.env文件
	if err := godotenv.Load(); err != nil {
		log.Println(".env file not found")
	}

	// 注册路由
	http.HandleFunc("/retrieval", errorHandler(ragflow.HandleRetrieval))

	// 启动服务器
	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
