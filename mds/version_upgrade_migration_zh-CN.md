# 版本升级迁移指南

本文档将“原理说明”和“日常执行指南”合并为一份中文参考，用于指导后续版本升级时的数据库迁移工作。

## 背景

软件持续迭代时，数据库表结构也会变化。如果新版本直接使用旧数据库，而没有明确的升级路径，就容易出现：

- 字段缺失
- 表结构不兼容
- 运行时 SQL 异常
- 旧数据语义和新逻辑不一致

为了解决这个问题，项目采用了基于应用版本驱动的启动迁移机制。

## 目标

这套迁移机制的目标是：

1. 所有版本迁移逻辑统一放在一个固定的 Go 文件中
2. 每个版本只实现“上一个版本 -> 当前版本”的迁移
3. 启动时自动判断是否发生升级
4. 跨多个版本升级时按顺序执行每一步迁移
5. 迁移完成后持久化当前版本
6. 避免把新安装误判为升级
7. 避免版本文件缺失时把数据库误判回 `1.0.0`
8. 明确阻止降级启动

## 核心文件

迁移机制主要涉及这些文件：

- [app_upgrade_migration.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/repository/app_upgrade_migration.go)
- [db.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/repository/db.go)
- [db_service.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/service/db_service.go)
- [database_service.dart](/Users/kidbei/projects/bot_sec_manager/lib/services/database_service.dart)
- [native_library_service.dart](/Users/kidbei/projects/bot_sec_manager/lib/services/native_library_service.dart)

## 版本状态持久化

当前实现会把版本状态存到两个位置：

1. 磁盘版本文件
   `Application Support/bot_sec_manager.version`
2. 数据库元信息表
   `app_metadata`

其中：

- 磁盘版本文件是主来源
- `app_metadata.runtime_version` 是兜底来源

这样既满足“通过磁盘文件判断升级”的原始要求，也能避免版本文件被删掉或写失败后把已升级过的数据库误判成旧版本。

## 启动流程

应用启动时，Flutter 会通过 FFI 向 Go 传入：

- 通过 `InitPathsFFI` 传入统一的应用基础目录
- `current_version`

Go 端的执行顺序如下：

1. 打开 SQLite
2. 通过 `PathManager` 从基础目录派生数据库路径和版本文件路径
3. 读取磁盘版本文件
4. 读取 `app_metadata.runtime_version`
5. 解析当前运行版本
6. 解析出数据库当前版本
7. 如果数据库版本低于当前版本，则执行迁移链
8. 用当前版本结构统一建表或校验表结构
9. 把当前版本写回：
   - 磁盘版本文件
   - `app_metadata`

Flutter 不再维护数据库路径或版本文件路径契约，只负责传入基础目录。所有运行时路径都由 core 从这一份基础目录统一派生。

## 版本判定规则

优先级从高到低如下：

1. 磁盘版本文件
2. `app_metadata.runtime_version`
3. 兼容性推断

如果版本文件和元信息都不存在：

- 数据库没有业务表：视为全新安装
- 数据库已有业务表：视为历史 `1.0.0` 数据库

这样才能区分：

- 新安装 `1.0.1` 不应触发 `1.0.0 -> 1.0.1`
- 从已发布 `1.0.0` 升级到 `1.0.1` 必须触发迁移

## 迁移规则

后续所有迁移都必须遵守以下规则：

1. 版本迁移只写在 [app_upgrade_migration.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/repository/app_upgrade_migration.go)
2. 一个版本只写一跳
3. 不允许跳版本写迁移
4. 不要把升级逻辑塞进建表函数
5. 建表函数只描述当前版本的最终结构
6. 迁移失败必须中断启动
7. 每次新增迁移都要补 repository/service 测试

## 多版本链式升级

迁移按链执行，例如：

- `1.0.0 -> 1.0.1`
- `1.0.1 -> 1.0.2`
- `1.0.2 -> 1.0.3`

如果用户从 `1.0.0` 直接升级到 `1.0.3`，系统必须依次执行：

1. `1.0.0 -> 1.0.1`
2. `1.0.1 -> 1.0.2`
3. `1.0.2 -> 1.0.3`

如果中间任何一步缺失，启动应直接失败，而不是猜测迁移路径。

## 当前特殊场景：`1.0.0 -> 1.0.1`

`1.0.1` 与 `1.0.0` 的数据库结构不兼容。

因此当前 `1.0.0 -> 1.0.1` 的策略是破坏性迁移：

- 删除所有业务表
- 再按当前版本结构重建

对应实现为 `migrateDatabaseFrom1_0_0To1_0_1`。

这样做的原因是：

- `1.0.0` 没有持久化版本状态
- 旧结构和新结构不兼容
- 继续依赖散落的列级兼容补丁，会让后续版本越来越难维护

## 如何新增一个迁移

假设当前发布版本是 `1.0.1`，下个版本是 `1.0.2`。

### 第一步：判断是否需要迁移

先问两个问题：

- 新代码能否直接使用旧数据库？
- 旧数据在新逻辑下语义是否仍然正确？

如果任一答案是否定的，就要加迁移。

### 第二步：注册新的一跳

在 `databaseVersionMigrations` 中新增一项：

```go
var databaseVersionMigrations = []databaseVersionMigration{
	{
		fromVersion: "1.0.0",
		toVersion:   "1.0.1",
		run:         migrateDatabaseFrom1_0_0To1_0_1,
	},
	{
		fromVersion: "1.0.1",
		toVersion:   "1.0.2",
		run:         migrateDatabaseFrom1_0_1To1_0_2,
	},
}
```

### 第三步：实现迁移函数

例如：

```go
func migrateDatabaseFrom1_0_1To1_0_2(db *sql.DB) error {
	// 1. 调整结构
	// 2. 搬运数据
	// 3. 清理废弃结构
	return nil
}
```

推荐做法：

- 结构完全不兼容：直接重建
- 数据必须保留：明确创建新结构并搬运数据
- 只是默认值或语义变化：执行针对性更新

不要做这些事情：

- 把多个版本的跳转混进一个函数
- 把迁移逻辑隐藏到 repository 初始化里
- 顺手加入和本次版本无关的“提前设计”

## 什么时候重建，什么时候搬运数据

适合重建的情况：

- 历史数据价值低
- 新旧结构差异过大
- 保留数据的成本明显高于收益
- 老版本没有稳定状态可安全推断

适合搬运数据的情况：

- 用户历史数据必须保留
- 旧结构和新结构之间有稳定映射
- 搬运结果可以被验证

如果选择搬运数据，建议顺序是：

1. 创建新结构
2. 转换并复制数据
3. 删除旧结构

## 当前版本结构也必须同步更新

迁移函数负责“如何从旧版本走到新版本”，但建表函数仍然必须反映当前版本的最终结构。

需要同步检查：

- [db.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/repository/db.go)
- 相关 repository SQL
- service 层对字段和结构的假设

原则是：

- 迁移负责过渡
- 建表负责最终态

## 不要再引入散落式隐式迁移

后续不要再回到下面这些做法：

- 在建表函数里偷偷 `ALTER TABLE ADD COLUMN`
- 在 repository 中埋字段重命名兼容
- 在多个模块里各写一点点临时迁移逻辑

这些做法会让升级路径变得不可追踪、不可验证、不可维护。

## 降级策略

如果已持久化版本高于当前运行版本，启动必须直接失败，并返回不支持降级的错误。

这样可以防止：

- 老代码误读新结构
- 低版本程序向高版本数据库写入不兼容数据
- 静默数据损坏

## 失败处理规则

如果迁移失败，必须：

- 返回错误
- 中断启动
- 不能吞掉错误继续运行

数据库迁移属于高风险操作，一旦失败后继续运行，很容易把数据库留在“半旧半新”的错误状态。

## 测试要求

每次新增迁移，至少覆盖以下场景：

1. 全新安装
2. 单步升级
3. 多步链式升级
4. 版本文件缺失但可以用 `app_metadata` 兜底
5. 降级启动被拒绝

测试主要补在：

- [app_upgrade_migration_test.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/repository/app_upgrade_migration_test.go)
- [db_service_test.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/service/db_service_test.go)

## 发布前检查清单

发布新版本前，请确认：

1. `pubspec.yaml` 版本号已更新
2. 新版本的一跳已注册到 `databaseVersionMigrations`
3. 迁移函数只处理单个版本跳转
4. 当前版本的表结构辅助函数已同步更新
5. repository/service 测试已补齐
6. 已验证新安装和升级两条路径
7. 本文档仍然准确

## 建议的提交说明

涉及迁移的提交建议明确写出目标版本和范围，例如：

```text
feat: add startup db migration for 1.0.1 -> 1.0.2
```

## 总结

后续所有数据库升级都应遵守一句话：

先识别版本，再执行单步迁移链，最后用当前版本结构兜底建表。

只要持续遵守这条规则，版本升级路径就会保持稳定、清晰、可维护。
