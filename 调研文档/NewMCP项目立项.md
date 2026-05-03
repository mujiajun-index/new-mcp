New MCP 项目立项

新一代统一大模型MCP服务。（类似skills，自主查找加载需要的MCP服务）
提供给大模型，一套统一的MCP 集合管理工具，大模型可查看 MCP分组，再查询分组下可用 实例 MCP 列表，根据实际情况自动调用相关 MCP。需求示例一：大模型可同时接管机器人分组下的多个类型机器人协议的MCP，（海陆空）下达指令控制。二：调用实时数据 MCP 服务，数据分析。最终实现，下达任务根据实时数据分析，自动更正命令，直到完成。

用户可创建 自定义 MCP 分组，管理 可用 MCP 服务。
视觉MCP配置：可自己配置图片识别模型。
外挂视觉系统：可自己配置图片识别模型+设置截图帧数+绑定摄像头。


1.第一版支持配置对接其他 mcp 并转换为通用MCP或流式长链接 MCP，兼容小智，或操作内网服务。如exa网络搜索MCP，配置好后可为，已注册链接的小智设备提供联网服务（长链接 MCP）

2.第二版用户支持配置本地 mcp对接平台，分为两种，第一种直连转发，第二种配置物联网参数平台启动MCP 服务。（接入智能家居）。

3.第三版完善付费相关，用户可调用平台公共 MCP，也可调用自建 MCP



参考资料：
NewAPI 统一大模型API平台  https://docs.newapi.pro/zh

MCP服务一键接入平台 闪猫 http://www.shanmaotech.cn/


https://github.com/thebigboy/dv-courses-mcp-xiaozhi

https://github.com/shadowcz007/mcp_server_exe

https://github.com/suhuail/api-mcp




我想要创建test目录，在其中设计最小实例，来测试 