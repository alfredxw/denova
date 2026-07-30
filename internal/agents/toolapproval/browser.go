package toolapproval

import (
	"encoding/json"
	"strings"

	"denova/config"
)

type browserArguments struct {
	Action  string `json:"action"`
	Command string `json:"command,omitempty"`
	URL     string `json:"url,omitempty"`
}

// evaluateBrowser separates passive browser reads from interactions that may
// change remote state. The browser uses one external-mutation descriptor for
// all actions, so its stable action schema is the narrowest reliable boundary.
func evaluateBrowser(request Request) Decision {
	if request.Mode == config.AgentApprovalYolo {
		return allow("browser_yolo", RiskHigh,
			"Yolo 模式允许此浏览器操作。 / Yolo mode allows this browser action.")
	}
	var input browserArguments
	if err := json.Unmarshal([]byte(request.Arguments), &input); err != nil {
		return prompt("browser_arguments_unknown", RiskHigh,
			"无法识别浏览器操作，需要你的确认。 / The browser action could not be classified and requires approval.")
	}
	action := strings.ToLower(strings.TrimSpace(input.Action))
	command := strings.ToLower(strings.TrimSpace(input.Command))
	switch action {
	case "close":
		return allow("browser_session_cleanup", RiskLow,
			"关闭本地浏览器会话不修改远端状态。 / Closing the local browser session does not change remote state.")
	case "open":
		if strings.TrimSpace(input.URL) == "" {
			return allow("browser_local_open", RiskLow,
				"打开空白的隔离标签页不访问网络。 / Opening an empty isolated tab does not access the network.")
		}
		return browserNavigationDecision(request.Mode)
	case "run":
		switch command {
		case "observe", "wait", "screenshot":
			return allow("browser_passive_read", RiskLow,
				"该浏览器操作只读取当前页面状态。 / This browser action only reads the current page state.")
		case "goto":
			return browserNavigationDecision(request.Mode)
		case "click", "fill", "type", "press", "select", "evaluate":
			return prompt("browser_remote_mutation", RiskHigh,
				"该浏览器交互可能提交数据或修改远端状态，需要你的确认。 / This browser interaction may submit data or change remote state and requires approval.")
		default:
			return prompt("browser_command_unknown", RiskHigh,
				"浏览器命令不在安全分类中，需要你的确认。 / The browser command has no safe classification and requires approval.")
		}
	default:
		return prompt("browser_action_unknown", RiskHigh,
			"浏览器操作不在安全分类中，需要你的确认。 / The browser action has no safe classification and requires approval.")
	}
}

func browserNavigationDecision(mode config.AgentApprovalMode) Decision {
	if mode == config.AgentApprovalAsk {
		return prompt("browser_network_access", RiskMedium,
			"浏览器将访问网络；Ask 模式需要你的确认。 / The browser will access the network; Ask mode requires approval.")
	}
	return allow("browser_network_read", RiskMedium,
		"Write 模式允许浏览器导航和网络读取。 / Write mode allows browser navigation and network reads.")
}
