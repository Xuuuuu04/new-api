package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type ModelTestToken struct {
	Id     int    `json:"id"`
	Name   string `json:"name"`
	Group  string `json:"group"`
	Status int    `json:"status"`
	UserId int    `json:"user_id"`
}

func GetModelTestTokens(c *gin.Context) {
	tokens, err := model.ListTokensForAdmin()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response := make([]ModelTestToken, 0, len(tokens))
	for _, token := range tokens {
		response = append(response, ModelTestToken{
			Id:     token.Id,
			Name:   token.Name,
			Group:  token.Group,
			Status: token.Status,
			UserId: token.UserId,
		})
	}
	common.ApiSuccess(c, response)
}

type ModelTestProxyRequest struct {
	TokenId int             `json:"token_id"`
	Payload json.RawMessage `json:"payload"`
}

func ProxyModelTestChatCompletions(c *gin.Context) {
	proxyModelTest(c, "/v1/chat/completions", nil)
}

func ProxyModelTestResponses(c *gin.Context) {
	proxyModelTest(c, "/v1/responses", nil)
}

func ProxyModelTestMessages(c *gin.Context) {
	proxyModelTest(c, "/v1/messages", func(req *http.Request, token *model.Token) {
		req.Header.Set("x-api-key", "sk-"+token.Key)
		if req.Header.Get("anthropic-version") == "" {
			req.Header.Set("anthropic-version", "2023-06-01")
		}
	})
}

type proxyHeaderSetter func(req *http.Request, token *model.Token)

func proxyModelTest(c *gin.Context, targetPath string, headerSetter proxyHeaderSetter) {
	var req ModelTestProxyRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.TokenId == 0 || len(req.Payload) == 0 {
		common.ApiError(c, errors.New("invalid params"))
		return
	}

	token, err := model.GetTokenById(req.TokenId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if token.Status != common.TokenStatusEnabled {
		common.ApiError(c, errors.New("token is not enabled"))
		return
	}

	targetURL := fmt.Sprintf("http://127.0.0.1:%s%s", getLocalPort(), targetPath)
	upstreamReq, err := http.NewRequest("POST", targetURL, bytes.NewReader(req.Payload))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer sk-"+token.Key)
	upstreamReq.Header.Set("Accept", "text/event-stream")
	upstreamReq.Header.Set("Cache-Control", "no-cache")
	if headerSetter != nil {
		headerSetter(upstreamReq, token)
	}

	resp, err := http.DefaultClient.Do(upstreamReq)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(c, resp)
	c.Status(resp.StatusCode)
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	flusher, _ := c.Writer.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := c.Writer.Write(buf[:n]); writeErr != nil {
				break
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return
		}
	}
}

func copyResponseHeaders(c *gin.Context, resp *http.Response) {
	for key, values := range resp.Header {
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
}

func isHopByHopHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade", "Content-Length":
		return true
	default:
		return false
	}
}

func getLocalPort() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return strconv.Itoa(*common.Port)
}
