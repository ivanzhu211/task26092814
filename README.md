# negaudit

付费搜索账户的否定关键词冲突审计库（仅标准库）。

## 用法

```go
acc, invalid, err := negaudit.LoadAccount(r) // r 为账户 JSON
conflicts := negaudit.Audit(acc)             // 确定性顺序的冲突记录
```

命令行：

```sh
go run ./cmd/negaudit account.json
```

每条冲突、每个非法条目各输出一行。

## 语义速查（关键词 \ 否定词）

|        | exact 否定        | phrase 否定                  | broad 否定          |
|--------|-------------------|------------------------------|---------------------|
| exact  | C: 词序列相等     | C: 否定词序列为关键词子序列  | C: set(N) ⊆ set(K)  |
| phrase | W: 词序列相等     | C: 同上                      | C: 同上             |
| broad  | W: 词序列相等     | C: 单词否定 ∈ K / W: 子序列  | C: 同上             |

C = critical（阻断关键词能匹配的一切搜索词），W = warning（仅阻断关键词
自身词序列构成的搜索词），空白 = 不报。

## 实现要点

- 否定词全局只存一份（共享列表不随挂载倍增），按 完全匹配词序列 /
  词组匹配词序列 / 广泛匹配词集合 建三个索引。
- 每个关键词枚举自身词序列的全部连续子序列与词集合的全部非空子集做
  索引查找，不做关键词 × 否定词的逐对比较。
- 满规模（5000 关键词 × 约 2 万条适用否定词）单次 Audit 约 35ms。

## 测试

```sh
go test ./...                              # 单元 + 对拍测试
go test -run '^$' -bench BenchmarkAuditFullScale .
```
