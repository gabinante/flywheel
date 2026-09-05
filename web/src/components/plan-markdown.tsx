import ReactMarkdown from 'react-markdown'
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { oneLight } from 'react-syntax-highlighter/dist/esm/styles/prism'
import rehypeSanitize, { defaultSchema } from 'rehype-sanitize'
import remarkGfm from 'remark-gfm'
import type { Components } from 'react-markdown'
import { MermaidBlock } from '@/components/mermaid-block'

const sanitizeSchema = {
  ...defaultSchema,
  tagNames: [
    ...(defaultSchema.tagNames ?? []),
    'table',
    'thead',
    'tbody',
    'tr',
    'th',
    'td',
    'del',
    'input',
  ],
  attributes: {
    ...defaultSchema.attributes,
    input: [
      ...(defaultSchema.attributes?.input ?? []),
      'type',
      'checked',
      'disabled',
    ],
  },
}

const headingBase = 'scroll-mt-4 font-sans text-foreground first:mt-0'

const planComponents: Components = {
  pre: ({ children }) => <>{children}</>,
  h1: ({ children }) => (
    <h1
      className={`${headingBase} mt-6 mb-2 text-lg font-semibold tracking-tight`}
    >
      {children}
    </h1>
  ),
  h2: ({ children }) => (
    <h2
      className={`${headingBase} mt-6 mb-2 text-base font-semibold tracking-tight`}
    >
      {children}
    </h2>
  ),
  h3: ({ children }) => (
    <h3
      className={`${headingBase} mt-5 mb-1.5 text-sm font-semibold tracking-tight`}
    >
      {children}
    </h3>
  ),
  h4: ({ children }) => (
    <h4
      className={`${headingBase} mt-4 mb-1 text-sm font-medium tracking-tight`}
    >
      {children}
    </h4>
  ),
  h5: ({ children }) => (
    <h5
      className={`${headingBase} mt-4 mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground`}
    >
      {children}
    </h5>
  ),
  h6: ({ children }) => (
    <h6
      className={`${headingBase} mt-4 mb-1 text-xs font-medium tracking-tight text-muted-foreground`}
    >
      {children}
    </h6>
  ),
  code(props) {
    const { className, children, ...rest } = props
    const match = /language-([\w-]+)/.exec(className ?? '')
    // react-markdown v10+ does not pass `inline`. Fenced blocks from mdast append a trailing
    // newline to the code value; inlineCode strips newlines. Unlabeled fences are still `pre > code`.
    const childText = String(children)
    const isBlockWithoutLang = !match && /\n/.test(childText)
    if (!match) {
      if (!isBlockWithoutLang) {
        return (
          <code
            className="box-decoration-clone rounded-md border border-white/10 bg-white/[0.06] px-1 py-px font-mono text-[0.8125em] leading-[inherit] text-foreground [overflow-wrap:anywhere] [word-break:break-word]"
                >
            {children}
          </code>
        )
      }
      return (
        <pre className="overflow-x-auto rounded-xl border border-white/10 bg-white/[0.03] p-2 font-mono text-xs backdrop-blur-sm">
          <code {...rest}>{children}</code>
        </pre>
      )
    }
    const raw = childText.replace(/\n$/, '')
    if (match[1] === 'mermaid') {
      return <MermaidBlock chart={raw} />
    }
    const lang = match[1]
    return (
      <SyntaxHighlighter
        language={lang}
        style={oneLight}
        PreTag="div"
        customStyle={{
          margin: 0,
          borderRadius: '0.375rem',
          fontSize: '0.75rem',
          lineHeight: 1.5,
        }}
        codeTagProps={{
          className: 'font-mono',
        }}
      >
        {raw}
      </SyntaxHighlighter>
    )
  },
}

type PlanMarkdownProps = {
  markdown: string
  className?: string
}

/** Renders a work stream plan: GFM, sanitized HTML, Prism for fenced code, Mermaid for mermaid fences. */
export function PlanMarkdown({ markdown, className }: PlanMarkdownProps) {
  if (!markdown.trim()) return null
  return (
    <div
      className={
        className ??
        'prose prose-sm dark:prose-invert max-w-none text-foreground [&_a]:text-primary [&_a]:underline [&_li]:my-0.5 [&_ol]:my-1 [&_p]:my-1 [&_p]:leading-relaxed [&_ul]:my-1 [&_table]:text-sm [&_td]:align-top [&_td]:break-words [&_th]:align-bottom'
      }
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[[rehypeSanitize, sanitizeSchema]]}
        components={planComponents}
      >
        {markdown}
      </ReactMarkdown>
    </div>
  )
}
