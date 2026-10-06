---
title: 奶屁龙
description: 奶屁龙的像素形象、矢量母版、配色与动作参考。
---

# 奶屁龙

一只圆肚子、短腿、爱开路的穴居小龙。薄荷色身体、象牙小角、方眼和蓝色折线尾鳍，共同构成它的识别轮廓。

## 形象参考

<div class="mascot-pair">
  <figure><img src="/mascots/dragon/reference.png" alt="已选定的奶屁龙像素参考原图" width="1312" height="1199" /><figcaption>原始参考 · 轮廓与比例依据</figcaption></figure>
  <figure><img src="/mascots/dragon/base.svg" alt="按参考图描出的奶屁龙纯路径 SVG" width="966" height="848" /><figcaption>纯路径 SVG · 可分层编辑</figcaption></figure>
</div>

- 头和身体连成圆滚轮廓，短腿贴近身体。
- 两只短象牙角，额头带浅色阶梯形 N 标记。
- 两只深色方眼，位置略有高低；小嘴、粉色舌头。
- 浅色肚皮和短爪，蓝色背鳍。
- 短而向外伸出的尾巴，末端是蓝色折线鳍。
- 身体保持干净简洁，动作道具只作为点缀。

### 固定色板

| 用途 | 色值 |
| --- | --- |
| 主体薄荷 | `#91DACD` |
| 身体阴影 | `#59AAA6` |
| 角、腹部、爪尖与 N 标记 | `#FFF3D7` |
| 浅色部位阴影 | `#F3DFB7` |
| 背鳍与尾鳍 | `#246FE2` |
| 蓝色阴影 | `#1657B8` |
| 描边与五官 | `#15212B` |
| 舌头与小范围腮红 | `#FB9B92` |

SVG 将参考图归并为平涂与少量阴影，保留主体轮廓、比例和像素边界。

## SVG 与图层

[下载标准 SVG](/mascots/dragon/base.svg) · [下载原始参考 PNG](/mascots/dragon/reference.png)

标准 SVG 的 `viewBox` 为 `225 206 966 848`，以等比例缩放使用。文件由可编辑的矢量路径组成。

| 图层 ID | 作用 |
| --- | --- |
| `body` | 身体主轮廓与阴影 |
| `tail` / `spines` | 尾巴与背鳍 |
| `horns` / `forehead` | 双角与 N 标记 |
| `belly` / `feet` | 肚皮与脚 |
| `paw-left` / `paw-right` | 两只前爪，可分别控制 |
| `eye-left` / `eye-right` / `mouth` | 可替换的五官 |
| `face-underlay` | 五官隐藏后填补底色，防止出现透明洞 |

母版沿参考图描出像素边界，以 4 个源坐标单位为最小描图步长，使用 `shape-rendering="crispEdges"`。小尺寸导出使用等比例缩放与最近邻采样。

## 多视图

<img src="/mascots/dragon/turnaround.svg" alt="奶屁龙正面、侧面、背面补全" width="648" height="176" />

[正面](/mascots/dragon/front.svg) · [侧面](/mascots/dragon/side.svg) · [背面](/mascots/dragon/back.svg)

正面突出双角、方眼与腹部；侧面呈现头部体积和尾巴走向；背面展示背鳍、长尾与身体轮廓。这组视图与上方标准姿态共同构成后续创作参考。

## WebUI 状态动作

WebUI 始终显示同一张 `base.svg`，服务状态只叠加两张透明 SVG 并逐帧切换。基础龙的身体、爪子和尾巴不移动，避免像素关节断开。

| 服务状态 | 两帧表现 |
| --- | --- |
| `ready` | 抬镐专注、落镐碎屑，表示正在开挖。 |
| `stopped` | 闭眼与单个 Z、闭眼与上浮的双 Z，表示打呼噜。 |
| `failed` | 红色感叹号、上跳感叹号与两侧警示点。 |
| `preparing` / `starting` / `stopping` | 蓝色单点、蓝色三点，表示状态正在切换。 |

状态层保持透明背景、4 单位像素步长和相同 `viewBox`；像素步长、关节衔接和停顿节奏的制作要求见[创作规范](./production)。
