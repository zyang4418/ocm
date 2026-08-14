/**
 * 轻量 Markdown 子集解析器:仅覆盖 OCM AI 回答实际出现的语法——
 * 标题、加粗、行内代码、无序/有序列表、表格、代码块、分隔线、链接。
 * 输出块级节点树,由 components/md-view 渲染。不引第三方库,控制包体积。
 */

export interface MdInline {
  type: 'text' | 'bold' | 'code' | 'link'
  text: string
  url?: string
}

export type MdBlock =
  | { type: 'p'; runs: MdInline[] }
  | { type: 'heading'; level: number; runs: MdInline[] }
  | { type: 'ul'; items: MdInline[][] }
  | { type: 'ol'; items: MdInline[][] }
  | { type: 'table'; header: MdInline[][]; rows: MdInline[][][] }
  | { type: 'code'; text: string }
  | { type: 'hr' }

export function parseMarkdown(src: string): MdBlock[] {
  const lines = (src || '').replace(/\r\n?/g, '\n').split('\n')
  const blocks: MdBlock[] = []
  let i = 0

  while (i < lines.length) {
    const trimmed = lines[i].trim()
    if (!trimmed) {
      i++
      continue
    }

    // 围栏代码块
    if (/^```/.test(trimmed)) {
      const buf: string[] = []
      i++
      while (i < lines.length && !/^```/.test(lines[i].trim())) {
        buf.push(lines[i])
        i++
      }
      i++ // 跳过结束围栏(可能不存在)
      blocks.push({ type: 'code', text: buf.join('\n') })
      continue
    }

    // 表格:当前行以 | 开头且下一行为分隔行
    if (trimmed.startsWith('|') && i + 1 < lines.length && isTableSep(lines[i + 1])) {
      const header = splitRow(trimmed).map(parseInline)
      const rows: MdInline[][][] = []
      i += 2
      while (i < lines.length && lines[i].trim().startsWith('|')) {
        const cells = splitRow(lines[i].trim()).map(parseInline)
        // 补齐到表头列数,避免渲染错位
        while (cells.length < header.length) cells.push([])
        rows.push(cells)
        i++
      }
      blocks.push({ type: 'table', header, rows })
      continue
    }

    // 无序列表
    if (/^[-*]\s+/.test(trimmed)) {
      const items: MdInline[][] = []
      while (i < lines.length) {
        const m = /^[-*]\s+(.*)$/.exec(lines[i].trim())
        if (!m) break
        items.push(parseInline(m[1]))
        i++
      }
      blocks.push({ type: 'ul', items })
      continue
    }

    // 有序列表
    if (/^\d+[.)]\s+/.test(trimmed)) {
      const items: MdInline[][] = []
      while (i < lines.length) {
        const m = /^\d+[.)]\s+(.*)$/.exec(lines[i].trim())
        if (!m) break
        items.push(parseInline(m[1]))
        i++
      }
      blocks.push({ type: 'ol', items })
      continue
    }

    // 标题
    const hm = /^(#{1,6})\s+(.*)$/.exec(trimmed)
    if (hm) {
      blocks.push({ type: 'heading', level: hm[1].length, runs: parseInline(hm[2]) })
      i++
      continue
    }

    // 分隔线
    if (/^(-{3,}|\*{3,}|_{3,})\s*$/.test(trimmed)) {
      blocks.push({ type: 'hr' })
      i++
      continue
    }

    // 段落:聚合到空行或下一个特殊块之前
    const buf: string[] = [trimmed]
    i++
    while (i < lines.length) {
      const t = lines[i].trim()
      if (!t || isBlockStart(t, i + 1 < lines.length ? lines[i + 1] : undefined)) break
      buf.push(t)
      i++
    }
    blocks.push({ type: 'p', runs: parseInline(buf.join('\n')) })
  }
  return blocks
}

/** 表格分隔行(如 |---|---|):每个单元格只含 - 与可选的 : */
function isTableSep(line: string): boolean {
  const t = (line || '').trim()
  if (t.indexOf('-') < 0) return false
  const cells = splitRow(t)
  return cells.length > 0 && cells.every((c) => /^:?-{1,}:?$/.test(c))
}

/** 切分表格行单元格(去掉首尾 | 后按 | 分割) */
function splitRow(line: string): string[] {
  let t = (line || '').trim()
  if (t.startsWith('|')) t = t.slice(1)
  if (t.endsWith('|')) t = t.slice(0, -1)
  return t.split('|').map((c) => c.trim())
}

/** 行首是否开启新的块级语法(用于段落聚合截止判断) */
function isBlockStart(trimmed: string, next: string | undefined): boolean {
  if (/^```/.test(trimmed)) return true
  if (/^(#{1,6})\s+/.test(trimmed)) return true
  if (/^[-*]\s+/.test(trimmed)) return true
  if (/^\d+[.)]\s+/.test(trimmed)) return true
  if (/^(-{3,}|\*{3,}|_{3,})\s*$/.test(trimmed)) return true
  if (trimmed.startsWith('|') && next !== undefined && isTableSep(next)) return true
  return false
}

/** 行内解析:代码 `x` > 加粗 **x** > 链接 [x](url),按最早出现位置顺序扫描 */
function parseInline(text: string): MdInline[] {
  const runs: MdInline[] = []
  let rest = text || ''
  while (rest.length > 0) {
    const codeIdx = rest.indexOf('`')
    const boldIdx = rest.indexOf('**')
    const linkIdx = rest.indexOf('[')
    const first = [codeIdx, boldIdx, linkIdx]
      .filter((idx) => idx >= 0)
      .sort((a, b) => a - b)[0]

    if (first === undefined) {
      runs.push({ type: 'text', text: rest })
      break
    }
    if (first > 0) runs.push({ type: 'text', text: rest.slice(0, first) })
    rest = rest.slice(first)

    if (first === codeIdx) {
      const end = rest.indexOf('`', 1)
      if (end < 0) {
        runs.push({ type: 'text', text: rest })
        break
      }
      if (end > 1) runs.push({ type: 'code', text: rest.slice(1, end) })
      rest = rest.slice(end + 1)
    } else if (first === boldIdx) {
      const end = rest.indexOf('**', 2)
      if (end < 0) {
        runs.push({ type: 'text', text: rest })
        break
      }
      if (end > 2) runs.push({ type: 'bold', text: rest.slice(2, end) })
      rest = rest.slice(end + 2)
    } else {
      const m = /^\[([^\]]*)\]\(([^)\s]*)\)/.exec(rest)
      if (!m) {
        // 孤立 [ 按纯文本吃掉,避免死循环
        runs.push({ type: 'text', text: rest.slice(0, 1) })
        rest = rest.slice(1)
        continue
      }
      runs.push({ type: 'link', text: m[1] || m[2], url: m[2] })
      rest = rest.slice(m[0].length)
    }
  }
  // 注意:不得在 run 之间插入零宽空格等字符——真机/模拟器引擎可能将其
  // 渲染出宽度,导致比纯文本多折行。跨 run 行断由 md-view 的嵌套 text
  // 结构(外层单 text 承载行流)天然保证,解析层只输出纯文本 run。
  return runs
}
