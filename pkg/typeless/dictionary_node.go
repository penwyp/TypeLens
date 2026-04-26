package typeless

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

//go:embed dictionary_node.js
var dictionaryNodeScript string

type nodeDictionaryRequest struct {
	Action       string `json:"action"`
	Offset       int    `json:"offset,omitempty"`
	Size         int    `json:"size,omitempty"`
	Term         string `json:"term,omitempty"`
	ID           string `json:"id,omitempty"`
	APIHost      string `json:"apiHost,omitempty"`
	UserDataPath string `json:"userDataPath,omitempty"`
	TimeoutMS    int64  `json:"timeoutMs,omitempty"`
}

func (c *DictionaryClient) runNodeRequest(ctx context.Context, request nodeDictionaryRequest, out ...any) error {
	request.APIHost = c.apiHost
	request.UserDataPath = c.userDataPath
	request.TimeoutMS = c.timeout.Milliseconds()

	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "node", "-e", dictionaryNodeScript)
	cmd.Stdin = bytes.NewReader(payload)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("调用 Typeless Node 桥接失败: %s", message)
	}

	if len(out) == 0 || out[0] == nil {
		return nil
	}
	if err := json.Unmarshal(stdout.Bytes(), out[0]); err != nil {
		return fmt.Errorf("解析 Typeless Node 返回失败: %w; body=%s", err, strings.TrimSpace(stdout.String()))
	}
	return nil
}
