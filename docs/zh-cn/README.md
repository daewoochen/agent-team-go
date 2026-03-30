# agent-team-go 中文说明

`agent-team-go` 是一个面向 Go 生态的 Agent Team 平台骨架，目标不是只展示“多智能体能跑”，而是从一开始就把下面这些能力做成一等公民：

- 自定义 Skill
- 自动安装缺失 Skill
- Feishu / Telegram 渠道接入
- 多 Agent 之间的结构化委派与协作
- Run replay、artifacts、事件日志

## 当前 MVP 能力

- 支持解析 `team.yaml`
- 支持 team 级别的模型 provider 配置，以及 agent 级别的 model 绑定
- 支持 `captain -> planner -> researcher/coder/reviewer` 的层级式协作
- 支持 `local`、`registry`、`git` 三类 Skill 来源
- 团队运行前自动检查并安装缺失 Skill
- 提供 `cli`、`telegram`、`feishu` 三类渠道配置模型
- 每次运行都会输出 replay log 到 `.agentteam/runs/`
- 每次运行都会输出 checkpoint 到 `.agentteam/checkpoints/`
- 支持 approval 事件和 work item 轨迹

## 快速开始

```bash
go run ./cmd/agentteam run \
  --team ./examples/software-team/team.yaml \
  --task "发布第一个公开 MVP，并保证首发体验足够强"
```

运行后你会看到：

- 团队摘要
- 结构化 delegation 事件
- artifacts 列表
- replay log 路径

你也可以查看模型配置说明：

```bash
go run ./cmd/agentteam models explain --team ./examples/software-team/team.yaml
```

或者查看某次运行的 replay：

```bash
go run ./cmd/agentteam replay show --run ./.agentteam/runs/<run-id>.json
```

## 核心概念

### TeamSpec

团队级定义，包含 agents、skills、channels、policies、budget。

### Skill

Skill 是可分发的能力包，当前约定包含：

- `skill.yaml`
- `prompt.md`
- 可选依赖与权限声明

### Delegation

Agent 之间不是“自由聊天”，而是通过结构化委派协议流转任务，明确：

- from / to
- task id
- deadline
- expected artifacts
- reason

### Channels

渠道层负责把飞书、Telegram、CLI 等输入统一成稳定的入口事件，并把 Team 的输出投递回去。

### Models

模型配置在 `team.yaml` 的 `models` 段里。推荐做法是：

1. API Key 放在环境变量里
2. 在 provider 里通过 `api_key_env` 引用
3. 每个 agent 通过 `model` 指定自己使用的模型

示例：

```yaml
models:
  default_model: openai/gpt-4.1-mini
  providers:
    openai:
      kind: openai-compatible
      base_url: https://api.openai.com/v1
      api_key_env: OPENAI_API_KEY

agents:
  - name: captain
    role: captain
    model: openai/gpt-4.1
```

运行前设置：

```bash
export OPENAI_API_KEY=your_api_key
go run ./cmd/agentteam models validate --team ./team.yaml
```

## 下一步演进

- 补齐真实 Telegram / Feishu 网关实现
- Skill Registry 改成可远程拉取的真实服务
- 接入 MCP
- 增强 replay 可视化
- 增加 sandbox 和审批流

你可以先从这些 case 开始体验：

- `examples/software-team/team.yaml`
- `examples/deep-research-team/team.yaml`
- `examples/incident-response-team/team.yaml`
- `examples/content-studio-team/team.yaml`

如果你准备把它做成一个真正能拿 star 的开源项目，建议优先持续完善三件事：

1. 让 demo 一键可跑
2. 让 README 首屏足够有记忆点
3. 让运行结果可回放、可解释、可分享
