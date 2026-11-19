# 棱版 Prism Plate

Go 二维打样渲染服务。HTTP 接收不可变稿本，输出具有内容摘要的 PNG 和 SVG，支持有界异步成像、校样裁决与可移交版面封包。领域为图形渲染，产品形态为 backend；依赖只有 Go 标准库。

## 启动

需要 Go 1.24 或更新版本；本次验证环境为 Go 1.27.0 / macOS arm64。演示流程还需要 Python 3 标准库。构建和运行无需网络。

```sh
go build -o bin/prismplate ./cmd/prismplate
./bin/prismplate -listen 127.0.0.1:7846 -data var/prism-plate-v1
```

```sh
curl -H 'Content-Type: application/json' --data-binary @examples/amber-proof.json http://127.0.0.1:7846/v1/plates
curl -X POST 'http://127.0.0.1:7846/v1/plates/amber-plate/render?revision=1&format=png'
```

第二个响应返回成品 id；以 GET /v1/artifacts/{id} 获取图片。服务不依赖浏览器、GPU、字体或外部图片。默认只监听回环地址，无身份认证；运行边界和容器挂载见 docs/operations.md。

## 场景约定

examples/amber-proof.json 提供曲线、线性渐变、仿射变换、镂空、裁域、二值蒙版和 screen 叠色。坐标原点在左上，颜色为 0..1 的 sRGB 直通色值，PNG 在内部转换为线性光预乘透明度。M/L/Q/C/Z 路径必须显式闭合，轮廓至少三条端点指令；Q/C 分别固定细分16/24段。图层可选 normal/multiply/screen，组透明度在整层合成后施加，蒙版与裁域处于版面坐标，笔刷与填充轮廓处于形状局部坐标。未提供的数值使用 JSON 的零值；形状必须明确传入可逆 transform。

PNG 是像素校样的权威结果：每像素四点覆盖采样，画布最大512×512。SVG 是复用展开几何的矢量校样；浏览器在覆盖率与透明渐变插值方面可能产生差异，不承诺逐像素相同。所有数值和引用均由服务生成，SVG不接受任意标记、脚本或外链。

## 渐变透明蒙版

图层的 `alpha_mask` 是本版新增的可选字段：它是形状数组，每项为 `{id, path, fill_rule, brush, transform}`，与普通形状使用同一套路径、笔刷和仿射规则。它整层渐隐，而不是逐形状裁切；`mask`/`clip` 保持原有二值语义不变。空数组或省略该字段表示不施加影响，旧稿本因此保持原义。

语义如下：

- 每个蒙版形状先按自身 `transform` 映射到版面画布，再取笔刷的透明度；`solid` 用 `color.a`，`linear` 用沿轴插值后的 `color.a`。RGB 不参与蒙版，仅保留以便继续编辑。
- 多个蒙版形状以 source-over 方式并集：`a = a_i + a_前·(1 - a_i)`。这个合并后的覆盖率再与图层 `opacity` 相乘一次，然后才把整层合成到画布，所以重叠区不会按形状个数被重复衰减。
- 蒙版形状与目标内容各自带 `transform`，可以互不相同：目标形状经逆矩阵在局部坐标判定，蒙版形状经自身矩阵在画布坐标判定，两者最终落在同一画布。因此移动内容不会带走蒙版，移动或旋转蒙版也不必改动目标形状。
- 与普通形状共享节点 id 命名空间；每层最多16个蒙版形状，轮廓与变换仍走同一准入和 1.6 亿边遍历预算，非法变换、重复 id 或坏渐变在写入前拒绝。

蒙版是稿本的一部分，随修订、分支、封包导入导出原样保留，可再编辑；渲染仍是对固定稿本的纯函数，相同稿本得到相同成品摘要。

## PNG 与 SVG 各自保证什么

`examples/fade-mask.json` 在透明背景上给出既可看又可核对的样例：一个渐变洗色蒙版加一个半透明三角形蒙版，两者在画布上重叠，目标层为渐变条与半透明曲线形状。`fade-mask.png` / `fade-mask.svg` 是由服务实际生成的对照输出，`fade-compare.png` 把同一图层蒙版前后并排（右侧为未蒙版参照），可确认只改变透明度、不改变色相。

- PNG 是权威像素结果：四点覆盖采样、线性光预乘合成，相同稿本与后端产生逐字节相同的 PNG。合成时透明度已乘过；导出为 8 位非预乘 NRGBA 时会做一次量化，因此低透明度像素可能出现 ±1 的颜色抖动。
- SVG 复用同一展开几何、同一蒙版变换与并集规则，输出 `<mask mask-type=alpha>`；蒙版形状用白色填充，即使查看器按亮度模式解释，结果也一致。但浏览器对渐变插值、抗锯齿与色彩空间的实现不同，SVG 只保证结构、位置、形状与蒙版关系，不承诺逐像素相同。
- 两者共同保证：蒙版在整层只衰减一次、重叠蒙版不重复相乘、蒙版与目标按各自变换定位到同一画布。逐像素相等只在 PNG 上声明。

```sh
curl -H 'Content-Type: application/json' --data-binary @examples/fade-mask.json http://127.0.0.1:7846/v1/plates
curl -X POST 'http://127.0.0.1:7846/v1/plates/fade-plate/render?revision=1&format=png'
python3 scripts/walkthrough.py --workflow fade
```

## 版本及生命周期

新建版面的初始稿本为1。PUT 传 expected_revision，只有当前最新版本相符才追加；旧版本不修改。归档禁止后续修订，仍可渲染历史及从历史分支。异步任务提交即捕获指定稿本；稍后修订不会改变它。任务只在当前进程内保留，重启后以已持久化成品为事实，不支持任务续跑；256条任务记录用尽后拒绝新任务。两名工作者、8条等待队列，所有渲染共用2个计算名额，同步或异步均可能在拥挤时得到拒绝。

取消是请求，已进入成品提交点的任务仍可成功；成功任务的事实不会变为取消。取消或失败不得提交半张图片。成品按版面、稿本、格式、内容摘要产生稳定标识；撤销后不允许再次悄悄激活。校样只有 open→accepted/rejected；通过的校样禁止撤销其成品。

## 入口

详见 docs/api.md。保存版面、修订、分支、归档、PNG、SVG、异步提交、取消、成品读取/撤销、校样、封包、恢复及写入隔离分别有独立的有界生产流程：

```sh
python3 scripts/walkthrough.py --workflow png
python3 scripts/walkthrough.py --workflow bundle
```

可选workflow值：create、revise、branch、archive、png、svg、fade、exposure、cancel、artifact、proof、bundle、restore、isolation、rejection。每个流程在临时数据根启动实际HTTP服务，完成后正常关停，不占用长期端口、不修改演示输入。

## 持久化与范围

完整状态以临时写入、文件fsync、原子替换提交，失败写入不会成为内存事实。数据根使用排他 writer.lock；干净退出会清除，异常退出后需人工确认没有旧进程再清除。摘要校验保护谱系一致性，不提供恶意篡改认证。启动还会重新编译持久化场景。封包只含版面稿本，不含成品、校样和任务；导入重名拒绝，不合并历史。

上线容量：128版面，每版64稿本，256成品，1024校样，状态快照128MiB，HTTP输入8MiB，单稿本4倍采样预计边遍历不超过1.6亿。容量达到时需要运维规划，不会自动淘汰被校样引用的数据。完整快照复制意味着吞吐适用于小规模打样服务。

自动化测试套件按初始化边界延后，本仓库不含测试命令。已提供构建、静态检查与真实生产流程演示；后续质量保障任务可以建立独立测试体系。Dockerfile提供容器构建路径，容器跨架构和生产部署不在本机Go构建的验证结论内。
