# 账号平台原地切换插件 (Account Platform Manager)

本插件为 Sub2API 的独立纯静态 UI 扩展，用于满足在同一账号 ID 内直接更换上游厂商与平台类型的需求。

## 背景与设计

1. **拒绝破坏性迁移**：
   - 当某个渠道厂商下架部分模型时，用户通常需要将其切换到其他平台（如从 Anthropic 切换为 OpenAI）。
   - 本插件调用宿主提供的 `admin.account.management.v1` Bridge 能力，保持原账号 ID、名称、备注、代理及调度权重不变，仅原地替换凭据与目标平台类型。

2. **升级不被覆盖**：
   - 插件作为独立的 `.s2plugin` 文件上传至宿主，宿主核心代码升级时不会覆盖插件的安装与界面。

3. **凭据安全与原子性**：
   - 切换时旧平台的密钥机密被彻底清除，不向前端泄露；
   - 自动清理旧平台的冷却、限流、错误状态和临时探测记录；
   - 支持多重敏感操作防护（二次验证 / TOTP Step-Up 守卫）与混合渠道风险二次确认。

## 构建与打包

在当前目录下执行 PowerShell 脚本打包：
```powershell
powershell -ExecutionPolicy Bypass -File ./build.ps1
```
打包后将生成 `account-platform-manager.s2plugin`，在 Sub2API 管理后台「插件管理」页面直接上传安装并启用即可。
