# GitHub 私有仓库

认证顺序：

1. `GITHUB_TOKEN` 或 `GH_TOKEN`；
2. 已安装 GitHub CLI 的 `gh auth token`；
3. Windows Credential Manager。

建议使用权限最小、有效期有限的 fine-grained token。不要把 Token 粘贴到 Skill、日志或命令参数中。

设置页的“验证当前凭据”会检查认证是否生效，并显示当前 REST API 剩余额度与重置时间。
公共仓库同样会使用该凭据，从而避免共享出口 IP 的低额度。更新检查使用五分钟内存缓存
及 ETag 条件请求；遇到限流时会显示恢复倒计时、保留上一次成功状态，并允许在配置凭据后
立即重试。
