# 故障排查

## 启动后窗口全白

0.1.0 早期构建在首次启动、尚无报告或事务时，可能把空集合传成
`null`，导致前端渲染失败。请使用最新发布包。新版同时提供错误提示页；
如果仍无法加载，请保留提示文字和 `data/logs` 下最新日志用于诊断。

## GUI 无法启动

- 确认 Windows 10/11 已安装 WebView2 Runtime；
- 运行 `csm doctor`；
- 检查自定义数据目录的写入权限。

## 私有仓库 401/404

- 运行 `gh auth status`；
- 检查 Token 是否有仓库 Contents 读取权限；
- 私有仓库无权限时 GitHub 可能返回 404。

## Skill 无法更新

- 检查是否存在本地修改；
- 检查来源锁中的仓库、路径和 ref；
- 重新生成计划，不要复用过期 plan ID。

## 更新失败

查看报告和事务 ID，再运行：

```powershell
csm rollback --transaction <ID>
```
