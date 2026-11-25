<p align="center">
  <img src="public/assets/kano-avatar.jpg" width="112" alt="鹿乃まほろ">
</p>

<h1 align="center">Kano Navi</h1>

<p align="center">
  <b>鹿乃在做什么？</b> 鹿乃まほろ的非官方粉丝近况整理站。
</p>

<p align="center">
  <a href="https://kano.fans/"><b>kano.fans</b></a>（日本服务器）·
  <a href="https://xiaolubao.com/"><b>xiaolubao.com</b></a>（中国大陆服务器，由 <a href="https://github.com/kanomahoro1224">鹿可可</a> 提供）
</p>

<p align="center">
  <a href="https://github.com/yexca/kano-navi/releases"><img alt="最新发布版本" src="https://img.shields.io/github/v/release/yexca/kano-navi"></a>
  <a href="https://hub.docker.com/r/yexca/kano-navi"><img alt="Docker Hub: yexca/kano-navi" src="https://img.shields.io/badge/docker-yexca%2Fkano--navi-2496ed?logo=docker&logoColor=white"></a>
  <a href="LICENSE"><img alt="许可证：AGPL-3.0" src="https://img.shields.io/github/license/yexca/kano-navi"></a>
</p>

<p align="center">
  <a href="README.md">English</a> · 简体中文 · <a href="README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="https://kano.fans/"><img src="docs/assets/readme/board.jpg" alt="桌面端与手机端展示效果"></a><br>
  <sub>日本服务器浅色主题截图，拍摄于 2026 年 11 月 19 日（日本时间）。</sub>
</p>

> [!NOTE]
> Kano Navi 是独立的粉丝项目，与鹿乃まほろ本人、所属事务所以及 X、YouTube、bilibili 或 LINE 均无关联，亦未获得官方认可。所有信息请始终以官方原始公告为准。

## 站点页面

| 页面          | 内容说明                                                                                           |
| ------------- | -------------------------------------------------------------------------------------------------- |
| **近况**      | 下一场直播的日本时间与本地时间倒计时、周日程与月日程视图、X 与 bilibili 的最新动态，以及最新视频。 |
| **动态**      | 收录 2010 年以来她的各个 X 账号、所属组合账号以及 bilibili 的全部动态，可按月份与账号浏览。        |
| **足迹**      | 记录自 2010 年首次翻唱投稿以来的活动节点，按活动身份分类，并附有带来源与出处的视觉档案。           |
| **作品**      | 她的专辑、单曲、同人作品及其各版本信息、参与作品，以及全部投稿歌曲。                               |
| **LINE BLOG** | 依原博客排版恢复的 2015–2019 年博客文章，包含照片与视频，并附中文与英文翻译。                      |

所有页面均支持英文、日文与简体中文，提供浅色与深色主题，并适配手机与桌面设备。日程条目会标注**自动识别**或**人工确认**，也可通过日历应用或订阅源跟踪日程与新视频。各页面的详细介绍见[使用指南](docs/user/index.md)（英文）。

<table>
  <tr>
    <td width="33%"><a href="docs/assets/readme/journey.jpg"><img src="docs/assets/readme/journey.jpg" alt="足迹页面"></a></td>
    <td width="33%"><a href="docs/assets/readme/works.jpg"><img src="docs/assets/readme/works.jpg" alt="作品页面"></a></td>
    <td width="33%"><a href="docs/assets/readme/lineblog.jpg"><img src="docs/assets/readme/lineblog.jpg" alt="LINE BLOG 存档"></a></td>
  </tr>
  <tr>
    <td align="center"><sub>足迹</sub></td>
    <td align="center"><sub>作品</sub></td>
    <td align="center"><sub>LINE BLOG</sub></td>
  </tr>
</table>

## 应用

- [Kano Navi Apple 版](https://github.com/yexca/kano-navi-apple)：由本站作者开发的 iOS、iPadOS 与 macOS 原生应用。
- [小鹿包们的宝物](https://dl.xiaolubao.com/)：由[鹿可可](https://github.com/kanomahoro1224)开发的 Android、鸿蒙、Windows 与 iOS 应用。

## 构建与运行

服务端按计划收集公开的动态与视频，从动态和周程图中识别日程条目，并将结果保存为本地快照。访客浏览时仅读取该快照，因此页面响应迅速，在原平台暂时不可访问时也能保留最近一次收集的内容。同一份快照也通过公开只读 API 与 MCP 向其他工具开放。

| 目标           | 推荐文档（英文）                                                                                            |
| -------------- | ----------------------------------------------------------------------------------------------------------- |
| 了解系统架构   | [工作原理](docs/how-it-works.md) 与 [系统架构](docs/architecture/index.md)                                  |
| 读取公开数据   | [公开 API](docs/architecture/public-api.md)                                                                 |
| 部署运行实例   | [Docker 部署](docs/operations/docker.md) 与 [配置说明](docs/operations/configuration.md)                    |
| 修改与开发代码 | [本地开发](docs/development/local-dev.md)、[开发文档](docs/development/index.md) 与 [Agent 指南](AGENTS.md) |
| 报告安全漏洞   | [安全说明](.github/SECURITY.md)                                                                             |
| 查找其他文档   | [文档索引](docs/README.md)                                                                                  |

## 说明与许可证

每一条动态、视频与活动均附有原始公开页面链接，并始终以原始页面为准。插画、艺人名称及其他第三方内容仍归原权利人所有，本项目代码许可证不授予对这些内容的任何权利；图片来源与出处分别记录在[足迹档案](docs/development/history-archive.md)、[作品合集](docs/development/works-archive.md)与 [LINE BLOG 存档](docs/development/lineblog-archive.md)中。

本项目由 [yexca](https://github.com/yexca) 创建。Kano Navi 是自由软件，采用 [GNU Affero 通用公共许可证 v3.0（AGPL-3.0）](LICENSE) 授权。
