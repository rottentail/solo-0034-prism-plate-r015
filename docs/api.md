# HTTP契约

所有写入场景的JSON入口拒绝未知字段和尾随内容，最大8MiB。服务返回JSON错误；404为对象不存在，409为版本或生命周期冲突，429为成像容量不足，400为不符合输入契约，504为超时。未知路由由HTTP框架返回404。

| 方法与路径 | 输入及结果 |
|---|---|
| GET /healthz | ready |
| POST /v1/plates | id,title,document →201版面 |
| GET /v1/plates/{id} | 完整稿本历史 |
| PUT /v1/plates/{id} | expected_revision,document →追加稿本 |
| POST /v1/plates/{id}/branch | revision,id,title →201独立版面 |
| POST /v1/plates/{id}/archive | 禁止修订；历史仍可读取 |
| POST /v1/plates/{id}/render?revision=1&format=png | 201成品元数据；format也可为svg |
| POST /v1/plates/{id}/exposures?revision=1&format=png | 202瞬态任务 |
| GET /v1/exposures/{id} | queued/running/succeeded/failed/cancelled |
| DELETE /v1/exposures/{id} | 202取消请求 |
| GET /v1/artifacts/{id} | 图片；ETag为成品字节摘要；X-Scene-Digest为稿本摘要 |
| DELETE /v1/artifacts/{id} | 撤销成品 |
| POST /v1/proofs | id,artifact →201 open校样 |
| PUT /v1/proofs/{id} | state为accepted或rejected，note为说明 |
| GET /v1/plates/{id}/bundle | 含完整稿本历史的封包JSON |
| POST /v1/bundles | 原样封包；201导入，重名409 |

成品响应中的data为null，避免重复传输二进制；GET成品获得实际内容。If-None-Match精确匹配强ETag时返回304。成功校样可从持久化快照确认，当前版本没有校样列表/详情接口。GET版面返回完整历史，服务没有全局列表接口。

## 图层渐变透明蒙版

`document.layers[].alpha_mask` 为可选数组，最多16项，每项：

```json
{"id":"fade-wash","path":[...],"fill_rule":"nonzero",
 "brush":{"kind":"linear","from":{"x":0,"y":0.5},"to":{"x":1,"y":0.5},
          "stops":[{"at":0,"color":{"r":1,"g":1,"b":1,"a":1}},
                   {"at":1,"color":{"r":1,"g":1,"b":1,"a":0}}]},
 "transform":[140,0,0,64,10,28]}
```

- 自身 `id` 与图层、形状共用命名空间；重复或不合法拒绝。
- `path`/`fill_rule`/`brush`/`transform` 的准入与普通形状一致；`brush` 只用 `a`，RGB 仅保留。
- 蒙版形状在画布空间求值（各自 `transform`），目标形状在局部空间求值（逆 `transform`），因此两者可分别变换而仍对齐同一画布。
- 多个蒙版形状按 source-over 并集，合并覆盖率再乘 `layer.opacity` 施加一次；重叠不重复衰减。
- 结果为 PNG 的 `<mask mask-type=alpha>` 与 SVG 的 `<mask>`；PNG 才是逐像素权威。
- 旧稿本无此字段，含义不变；这是稿本中可编辑、可随修订与封包移交的数据。
