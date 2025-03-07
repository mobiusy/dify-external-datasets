package ragflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// RetrievalRequest 定义检索请求的结构
type RetrievalRequest struct {
	KnowledgeID      string           `json:"knowledge_id"`
	Query            string           `json:"query"`
	RetrievalSetting RetrievalSetting `json:"retrieval_setting"`
}

// RetrievalSetting 定义检索设置
type RetrievalSetting struct {
	TopK           int     `json:"top_k"`
	ScoreThreshold float64 `json:"score_threshold"`
}

// Record 定义检索记录的结构
type Record struct {
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Score    float64                `json:"score"`
	Title    string                 `json:"title"`
	Content  string                 `json:"content"`
}

// RetrievalResponse 定义API响应的结构
type RetrievalResponse struct {
	Records []Record `json:"records"`
}

// ErrorResponse 定义错误响应的结构
type ErrorResponse struct {
	ErrorCode int    `json:"error_code"`
	ErrorMsg  string `json:"error_msg"`
}

// CustomError 自定义错误类型
type CustomError struct {
	Code    int
	Message string
}

func (e *CustomError) Error() string {
	return e.Message
}

// validateAPIKey 验证API密钥
func validateAPIKey(authHeader string) error {
	if authHeader == "" {
		return &CustomError{1001, "缺少Authorization头"}
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return &CustomError{1001, "无效的Authorization头格式。预期格式为'Bearer <api-key>'"}
	}

	return nil
}

// validateRequest 验证请求的合法性
func validateRequest(r *http.Request) (*RetrievalRequest, error) {
	if r.Method != http.MethodPost {
		return nil, &CustomError{405, "Method not allowed"}
	}

	var req RetrievalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, &CustomError{400, "无效的请求格式"}
	}

	if req.KnowledgeID == "" {
		return nil, &CustomError{2001, "知识库不存在"}
	}

	return &req, nil
}

// callExternalAPI 调用外部检索API
func callExternalAPI(req *RetrievalRequest, authHeader string) ([]byte, error) {
	apiReq := struct {
		Question            string   `json:"question"`
		DatasetIds          []string `json:"dataset_ids"`
		DocumentIds         []string `json:"document_ids,omitempty"`
		Page                int      `json:"page,omitempty"`
		PageSize            int      `json:"page_size,omitempty"`
		TopK                int      `json:"top_k,omitempty"`
		SimilarityThreshold float64  `json:"similarity_threshold,omitempty"`
		VectorSimilarity    float64  `json:"vector_similarity,omitempty"`
		RerankId            string   `json:"rerank_id,omitempty"`
		Keyword             bool     `json:"keyword,omitempty"`
		Highlight           bool     `json:"highlight,omitempty"`
	}{
		Question:   req.Query,
		DatasetIds: []string{req.KnowledgeID},
		// TopK:                20,
		SimilarityThreshold: req.RetrievalSetting.ScoreThreshold,
		Page:                1,
		PageSize:            req.RetrievalSetting.TopK,
	}

	reqBody, err := json.Marshal(apiReq)
	if err != nil {
		return nil, &CustomError{500, "内部服务器错误"}
	}

	url := os.Getenv("API_URL") + "/api/v1/retrieval"
	request, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, &CustomError{500, "内部服务器错误"}
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", authHeader)

	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return nil, &CustomError{500, "调用检索服务失败"}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &CustomError{500, "读取响应失败"}
	}

	if resp.StatusCode != http.StatusOK {
		var apiError struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(respBody, &apiError); err != nil {
			return nil, &CustomError{500, "解析错误响应失败"}
		}
		return nil, &CustomError{apiError.Code, apiError.Message}
	}

	return respBody, nil
}

// processAPIResponse 处理API响应
func processAPIResponse(respBody []byte) (*RetrievalResponse, error) {
	// 首先尝试解析通用响应格式
	var generalResp struct {
		Code    int         `json:"code"`
		Data    interface{} `json:"data"`
		Message string      `json:"message"`
	}

	if err := json.Unmarshal(respBody, &generalResp); err != nil {
		return nil, &CustomError{500, "解析响应失败"}
	}

	// 检查是否存在错误状态
	if generalResp.Code != 0 && generalResp.Code != 200 {
		return nil, &CustomError{generalResp.Code, generalResp.Message}
	}

	// 如果没有错误，继续解析完整的响应结构
	var apiResp struct {
		Code int `json:"code"`
		Data struct {
			Chunks []struct {
				Content          string  `json:"content"`
				DocumentId       string  `json:"document_id"`
				DocumentKeyword  string  `json:"document_keyword"`
				Highlight        string  `json:"highlight"`
				Similarity       float64 `json:"similarity"`
				TermSimilarity   float64 `json:"term_similarity"`
				VectorSimilarity float64 `json:"vector_similarity"`
			} `json:"chunks"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, &CustomError{500, "解析响应失败"}
	}

	response := RetrievalResponse{
		Records: make([]Record, len(apiResp.Data.Chunks)),
	}

	apiUrl := os.Getenv("API_URL")
	for i, chunk := range apiResp.Data.Chunks {
		// get file extension from chunk.DocumentKeyword field
		extension := ""
		if strings.Contains(chunk.DocumentKeyword, ".") {
			parts := strings.Split(chunk.DocumentKeyword, ".")
			extension = parts[len(parts)-1]
		}
		response.Records[i] = Record{
			Metadata: map[string]interface{}{
				// /document/7f787010f42411ef9fe10242ac180006?ext=pdf&prefix=document
				"path":        fmt.Sprintf("%s/document/%s?ext=%s&prefix=document", apiUrl, chunk.DocumentId, extension),
				"description": chunk.DocumentKeyword,
			},
			Score:   chunk.Similarity,
			Title:   chunk.DocumentKeyword,
			Content: chunk.Content,
		}
	}

	return &response, nil
}

// HandleRetrieval 处理/retrieval端点的请求
func HandleRetrieval(w http.ResponseWriter, r *http.Request) error {
	// 验证API密钥
	if err := validateAPIKey(r.Header.Get("Authorization")); err != nil {
		return err
	}

	// 验证请求
	req, err := validateRequest(r)
	if err != nil {
		return err
	}

	// 调用外部API
	respBody, err := callExternalAPI(req, r.Header.Get("Authorization"))
	if err != nil {
		return err
	}

	// 处理响应
	response, err := processAPIResponse(respBody)
	if err != nil {
		return err
	}

	// 返回响应
	return json.NewEncoder(w).Encode(response)
}
