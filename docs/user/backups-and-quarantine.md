# 备份、隔离与跨盘恢复

Codex Skill Manager 在覆盖或移除个人 Skill 前，会先通过同盘移动保存原内容。这样既
避免执行仓库脚本，也能在事务失败后可靠回滚。

当 Skills 根目录与应用配置的备份或隔离目录位于同一磁盘时，内容保存在配置目录中。
当两者位于不同磁盘（例如 Skills 在 `C:`、便携版应用数据在 `D:`）时，Windows 无法
使用原子重命名跨盘移动。应用会自动改用：

- `<SkillsRoot>/.csm-backups/<skill>/<transaction>/content`
- `<SkillsRoot>/.csm-quarantine/<skill>/<transaction>/content`

事务日志记录实际保存路径，恢复和回滚会读取该路径，不依赖默认目录推测。上述内部
目录不会作为普通 Skill 显示，也不会被安全扫描重复计入。

不要手动修改或移动仍处于可恢复状态的事务目录。如果需要释放空间，应先在应用中
确认对应事务不再需要恢复；永久清理由使用者显式完成。
