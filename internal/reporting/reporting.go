package reporting

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/model"
)

func WriteScan(root string, report model.ScanReport) (string, string, error) {
	base := fmt.Sprintf("%s_scan_%s", time.Now().Format("2006-01-02_150405"), sanitize(filepath.Base(report.Target)))
	jsonPath := filepath.Join(root, base+".json")
	mdPath := filepath.Join(root, base+".md")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := atomicWrite(jsonPath, data); err != nil {
		return "", "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Skill 安全扫描报告\n\n")
	fmt.Fprintf(&b, "- 目标：`%s`\n", report.Target)
	fmt.Fprintf(&b, "- 状态：%s\n", report.Status)
	fmt.Fprintf(&b, "- 最高风险：%s\n", report.HighestSeverity)
	fmt.Fprintf(&b, "- 当前未忽略风险：%s（%d 条，已忽略 %d 条）\n", report.ActiveHighestSeverity, report.ActiveFindingCount, report.IgnoredFindingCount)
	fmt.Fprintf(&b, "- 扫描文件：%d\n", report.FilesScanned)
	fmt.Fprintf(&b, "- 扫描器：%s\n\n", report.ScannerVersion)
	if report.CodexReview != nil {
		fmt.Fprintf(&b, "## Codex 辅助复核\n\n- 状态：%s\n- 模型：%s\n- 推理强度：%s\n- 总结：%s\n\n",
			report.CodexReview.Status, report.CodexReview.Model, report.CodexReview.ReasoningEffort, report.CodexReview.Summary)
	}
	if len(report.Findings) == 0 {
		b.WriteString("未发现已知风险模式。该结果不构成安全保证。\n")
	} else {
		b.WriteString("## 风险簇\n\n")
		for _, cluster := range report.Clusters {
			fmt.Fprintf(&b, "### %s · %s\n\n- 规则：%s\n- 文件类型：%s\n- 原始命中：%d\n- 影响文件：%d\n- 确定性底线：%t\n- 已人工忽略：%t\n",
				cluster.Severity, cluster.Title, cluster.RuleID, cluster.FileClass, cluster.FindingCount,
				len(cluster.AffectedFiles), cluster.Deterministic, cluster.Ignored)
			if cluster.IgnoreReason != "" {
				fmt.Fprintf(&b, "- 人工决定原因：%s\n", cluster.IgnoreReason)
			}
			b.WriteString("\n")
		}
		b.WriteString("## 发现\n\n")
		for _, f := range report.Findings {
			ignored := "否"
			if f.Ignored {
				ignored = "是"
			}
			fmt.Fprintf(&b, "### %s · %s\n\n- 规则：%s\n- 文件：`%s:%d`\n- 说明：%s\n- 命中内容：`%s`\n- 建议：%s\n- 已忽略：%s\n",
				f.Severity, f.Title, f.RuleID, f.File, f.Line, f.Explanation, strings.ReplaceAll(f.Evidence, "`", "'"), f.Recommendation, ignored)
			if f.IgnoreReason != "" {
				fmt.Fprintf(&b, "- 忽略原因：%s\n", f.IgnoreReason)
			}
			b.WriteString("\n")
		}
	}
	if err := atomicWrite(mdPath, []byte(b.String())); err != nil {
		return "", "", err
	}
	return mdPath, jsonPath, nil
}

func WriteTransaction(root string, tx model.Transaction) (string, string, error) {
	base := fmt.Sprintf("%s_%s_%s", time.Now().Format("2006-01-02_150405"), tx.Type, sanitize(tx.ID))
	jsonPath := filepath.Join(root, base+".json")
	mdPath := filepath.Join(root, base+".md")
	data, err := json.MarshalIndent(tx, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := atomicWrite(jsonPath, data); err != nil {
		return "", "", err
	}
	md := fmt.Sprintf("# Skill 管理事务\n\n- ID：`%s`\n- 类型：%s\n- 状态：%s\n- 目标：%s\n- 开始：%s\n- 完成：%s\n- 错误：%s\n",
		tx.ID, tx.Type, tx.Status, strings.Join(tx.Targets, "、"),
		tx.StartedAt.Local().Format(time.RFC3339), formatTime(tx.CompletedAt), tx.Error)
	if err := atomicWrite(mdPath, []byte(md)); err != nil {
		return "", "", err
	}
	return mdPath, jsonPath, nil
}

func AppendEvent(root, eventType string, value any) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	event := struct {
		Timestamp time.Time `json:"timestamp"`
		Type      string    `json:"type"`
		Data      any       `json:"data"`
	}{
		Timestamp: time.Now().UTC(),
		Type:      eventType,
		Data:      value,
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	path := filepath.Join(root, time.Now().Format("2006-01-02")+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, s)
	return strings.Trim(s, "-")
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format(time.RFC3339)
}
