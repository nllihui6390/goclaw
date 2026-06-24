import { defineStore } from 'pinia'

export const useToolsStore = defineStore('tools', () => {
  const toolMeta = {
    // 基础工具
    manage_config:     { icon: '⚙️', desc: '管理配置文件' },
    exec:              { icon: '⚡', desc: 'Shell 命令执行（带安全守卫）' },
    list_files:        { icon: '📂', desc: '列出目录文件' },
    write_file:        { icon: '📝', desc: '写入文件' },
    read_file:         { icon: '📖', desc: '读取文件' },
    edit_file:         { icon: '✏️', desc: '编辑文件（精确字符串替换）' },
    append_file:       { icon: '➕', desc: '追加文件内容' },
    send_file:         { icon: '📦', desc: '发送文件给用户' },
    browser_use:       { icon: '🌐', desc: '浏览器自动化 (rod)' },
    set_user_timezone: { icon: '🌍', desc: '设置用户时区' },
    // 系统/状态类
    network_check:     { icon: '📶', desc: '网络连通性检查' },
    // 网络/信息类
    http_request:      { icon: '🌐', desc: '发送 HTTP 请求' },
    url_summary:       { icon: '📄', desc: '网页正文摘要' },
    web_search:        { icon: '🔍', desc: '网页搜索' },
    weather:           { icon: '🌤️', desc: '天气查询（和风/OpenWeather/Seniverse）' },
    // 计算/代码类
    calculate:         { icon: '🔢', desc: '数学计算' },
    run_code:          { icon: '💻', desc: '执行代码片段' },
    // 文档/图片类
    read_pdf:          { icon: '📄', desc: '读取 PDF 文件' },
    ocr_image:         { icon: '👁️', desc: '图片文字识别 (OCR)' },
    generate_image:    { icon: '🖼️', desc: '生成图片' },
    agnes_image:       { icon: '🎨', desc: 'AI图像生成（文生图、图生图、图像编辑、多图合成）' },
    agnes_video:       { icon: '🎬', desc: 'AI视频生成（文生视频、图生视频、多图视频、关键帧动画）' },
    // 数据库类
    database_query:    { icon: '🗄️', desc: '数据库查询' },
  }

  /** 所有工具名列表 */
  const allToolNames = Object.keys(toolMeta)

  /** 获取工具元信息 */
  function getTool(name) {
    return toolMeta[name] || { icon: '🔧', desc: '' }
  }

  return { toolMeta, allToolNames, getTool }
})
