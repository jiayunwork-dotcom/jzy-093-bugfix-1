# rocalc — 苦咸水反渗透（RO）膜过程核算服务

一个常驻的 HTTP 计算服务，只管反渗透里的两件事：

1. **渗透压**：van't Hoff 关系 π = i·C·R·T；
2. **膜通量与盐衡算**：净推动力 NDP、产水流量 Qp、回收率 Y、浓水浓度 Cb、全系统盐物料守恒。

没有页面，不是水费账单或工单系统。给它一份**具名工况档**（可反复点名计算），或临时拼一份工况直接提交，都返回完整结果；工况不合法时带稳定错误码和中文原因返回。

## 物理模型与单位约定（全服务统一，不做隐式量纲换算）

| 量 | 单位 |
|---|---|
| 压力：渗透压、工作压差、NDP | **bar** |
| 摩尔浓度 | mol/L |
| 质量浓度 | g/L；摩尔质量 g/mol |
| 温度 | K（必须为正） |
| 流量 | L/h |
| 膜面积 | m²；渗透系数 Lp | LMH = L/(m²·h)/bar |

气体常数 R = 0.08314 L·bar/(mol·K)。

公式：

- 渗透压：`π = i·C·R·T`（NaCl 取 i=2）。浓度可直接给 mol/L，也可给 g/L + g/mol 换算；两种口径同时给时必须自洽（相对容差 1e-3），否则拒绝。
- 净推动力：`NDP = Δp − β·πf + πp`，β 为浓差极化因子（β≥1，1 表示无极化）。这里的 Δp 是**已经扣过产水背压的工作压差**，全链路只在 bar 量纲下相减。
- 产水流量：`Qp = A·Lp·NDP`（L/h）。
- 回收率：`Y = Qp/Qf`，必须落在**开区间 (0,1)**。
- 产水浓度：完全截留 r=1 时 Cp=0；r<1 时 Cp = Cf·(1−r)。
- 浓水浓度（由盐守恒推导，回收越高浓水越浓）：
  - r=1 极限：`Cb = Cf/(1−Y)`
  - r<1：`Cb = Cf·[1−(1−r)·Y]/(1−Y)`
- 物料守恒：`Cf·Qf = Cp·Qp + Cb·Qb`，其中 Qb = Qf − Qp。服务对每个结果都做闭合校验（摩尔口径必校，给了摩尔质量再校质量口径），不闭合直接报错。

> 明确防守的坑：浓水浓度**绝不能**写成 `Cf·(1−Y)`——那样回收越高浓水反而越稀，盐守恒必然崩塌。

## 非法工况（HTTP 400，带 code + reason）

- 浓度为负 / 温度不为正 / van't Hoff 因子不为正 / 质量与摩尔口径不自洽；
- 工作压差、进料流量、膜面积、渗透系数不为正，极化因子 <1，截留率不在 (0,1]；
- `non_positive_ndp`：工作压差已低于膜面渗透压（NDP<0），无论膜面积、渗透系数多大，**绝不报告正通量**；
- `invalid_recovery`：回收率 Y 不在 (0,1)；
- `salt_balance_violation`：进料盐量 ≠ 产水盐量 + 浓水盐量。

交叉关系也有测试钉住：升温则 π 升、同压差下 NDP 降；回收上升则 Cb 单调上升；r=1 时产水零盐；Δp 恰好等于 πf 且无极化时 Qp 精确为 0。

## 内置苦咸水工况档 `brackish-default`

进料 5 g/L NaCl（M=58.44 g/mol），25 ℃，i=2；Δp=12 bar，Qf=1000 L/h，A=36 m²，Lp=1.8 LMH/bar，r=1，β=1。

手算对照（服务实跑一致）：

| 量 | 手算 | 服务结果 |
|---|---|---|
| C | 5/58.44 = 0.08556 mol/L | 0.08556 mol/L |
| πf | 2×0.08556×0.08314×298.15 | **4.2416 bar**（几个巴量级） |
| NDP | 12 − 4.2416 | 7.7584 bar |
| Qp | 36×1.8×7.7584 | 502.74 L/h |
| Y | 502.74/1000 | 0.5027 |
| Cb | Cf/(1−Y) | 0.17206 mol/L = 10.055 g/L |
| 盐守恒 | 进料 5000 g/h = 0 + 浓水 5000 g/h | 残差 ≈ 1e-12 |

## HTTP 接口

固定监听 `:8080`（可用环境变量 `RO_PORT` 覆盖）。JSON 请求/响应，字段名自带单位。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 健康检查 |
| GET | `/v1/constants` | R 取值与全部公式 |
| GET | `/v1/cases` | 列出已登记工况档 |
| POST | `/v1/cases` | 登记具名工况档（重名 409） |
| PUT | `/v1/cases/{name}` | 登记或整体替换 |
| GET | `/v1/cases/{name}` | 取回工况档 |
| DELETE | `/v1/cases/{name}` | 删除 |
| POST | `/v1/cases/{name}/evaluate` | 点名计算 |
| POST | `/v1/evaluate` | 临时工况直接算，无需登记 |

点名内置档：

```bash
curl -s -X POST http://localhost:8080/v1/cases/brackish-default/evaluate
```

临时工况（部分截留 r=0.98，产水带盐，自动校验守恒）：

```bash
curl -s -X POST http://localhost:8080/v1/evaluate -H 'Content-Type: application/json' -d '{
  "feed": {"mass_concentration_g_per_l": 5, "molar_mass_g_per_mol": 58.44,
           "temperature_k": 298.15, "vanth_hoff_factor": 2},
  "applied_pressure_bar": 12, "feed_flow_lh": 1000,
  "permeability_lmh_per_bar": 1.8, "area_m2": 36,
  "salt_rejection": 0.98}'
```

错误响应示例：

```json
{"error":"non_positive_ndp: ...","code":"non_positive_ndp","reason":"工作压差 3 bar 低于膜面渗透压 4.24 bar ..."}
```

## 代码结构（按职责拆包）

```
cmd/server/              启动入口（信号优雅关停）
internal/osmotic/        van't Hoff 渗透压、质量/摩尔浓度换算
internal/membrane/       types.go 输入输出类型
                         flux.go    NDP、Qp、回收率、Evaluate 编排
                         balance.go 浓水浓度、产水浓度、盐守恒校验
internal/cases/          具名工况档登记/取用（带锁、取值返副本）、内置苦咸水档
internal/validation/     输入合法性检查与稳定错误码
internal/httpapi/        net/http 接口、DTO、错误码到 HTTP 状态码映射
```

登记表按名取**副本**计算、不缓存结果，因此同一进程里两份工况档的结果各自独立，不会串名或互相污染（有并发测试覆盖）。

## 开发与测试

```bash
go test ./... -count=1     # 全部自动化测试
go test -race ./...        # 竞态检测
go vet ./... && gofmt -l . # 静态检查与格式
go run ./cmd/server        # 本地启动 :8080
```

测试重点：
- `TestEvaluate_BelowOsmoticPressure_NoPositiveFlux` —— 压差低于渗透压时绝不冒正通量；
- `TestEvaluate_PartialRejection_SaltBalance` / `TestCheckBalance_MolarClosure` —— 进料盐量 = 产水 + 浓水，且错写成 Cf·(1−Y) 必被守恒校验抓住；
- `TestRegistry_TwoCasesStayIndependent` —— 两份工况档互不串档。

## Docker

多阶段构建：构建阶段先 `go test ./...`（测试随仓库一起在容器内跑通），再静态编译；运行阶段用 distroless 非 root 镜像，暴露固定端口 8080。

```bash
docker build -t rocalc:latest .
docker run --rm -p 8080:8080 rocalc:latest
```

Go 1.22，仅用标准库，无第三方依赖。
