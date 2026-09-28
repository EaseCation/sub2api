//go:build unit

package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGroupSessionSkillMiddleware(t *testing.T) {
	for _, tc := range []struct {
		path, body, promptPath string
	}{
		{"/v1/responses", `{"input":"task"}`, "instructions"},
		{"/backend-api/codex/responses", `{"input":"task"}`, "instructions"},
		{"/v1/messages", `{"messages":[]}`, "system"},
		{"/v1/chat/completions", `{"messages":[]}`, "messages.0.content"},
		{"/v1beta/models/gemini:streamGenerateContent", `{"contents":[]}`, "systemInstruction.parts.0.text"},
		{"/v1/images/generations", `{"prompt":"image"}`, ""},
		{"/v1/responses/compact", `{"input":[]}`, ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: &service.Group{SessionSkillEnabled: true, SessionSkill: "purpose check"}})
				// Earlier middleware may have preread the body; consumers must get
				// the rewritten version through both supported read paths.
				body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
				require.NoError(t, err)
				requestmodel.ResetRequestBody(c.Request, body)
			})
			r.Use(GroupSessionSkill())
			r.POST(tc.path, func(c *gin.Context) {
				body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
				require.NoError(t, err)
				if tc.promptPath == "" {
					require.Equal(t, tc.body, string(body))
				} else {
					require.Contains(t, gjson.GetBytes(body, tc.promptPath).String(), "purpose check")
					stream, err := io.ReadAll(c.Request.Body)
					require.NoError(t, err)
					require.Equal(t, string(body), string(stream))
				}
				// A composite target must not replace the source group's policy.
				c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: &service.Group{SessionSkillEnabled: true, SessionSkill: "target"}})
				require.Equal(t, "purpose check", service.GroupSessionSkillFromContext(c))
				c.Status(http.StatusNoContent)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)))
			require.Equal(t, http.StatusNoContent, w.Code)
		})
	}
}

func TestGroupSessionSkillMiddlewareDisabledDoesNotReadBody(t *testing.T) {
	r := gin.New()
	r.Use(GroupSessionSkill())
	r.POST("/v1/responses", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.Equal(t, "untouched", string(body))
		c.Status(http.StatusNoContent)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("untouched")))
	require.Equal(t, http.StatusNoContent, w.Code)
}
