// Resource-specific source boundaries supplement the repository's secret audit.
export function auditResourceWeb(entries) {
  const sink = 'apps/web/src/components/Markdown.astro';
  let sinks = 0;
  for (const [path, content] of entries) {
    if (!path.startsWith('apps/') || !/\.(?:astro|[cm]?[jt]sx?)$/.test(path)) continue;
    const count = [...content.matchAll(/\bset:html\s*=/g)].length;
    if (path.startsWith('apps/web/') && count) {
      sinks += count;
      if (path !== sink || count !== 1 || !content.includes('const html = renderMarkdown(Astro.props.source);') || !content.includes('set:html={html}') || !content.includes("from '../lib/markdown.server'")) throw new Error(`Unapproved HTML sink: ${path}`);
    }
    // Includes static/dynamic imports and re-exports. Server modules cannot be
    // re-exported through ordinary TS helpers into a React/browser component.
    const imports = text => [...text.matchAll(/(?:from\s*|import\s*\(?\s*|require\s*\(\s*)['"][^'"\n]*\.server(?:\.[cm]?[jt]sx?)?['"]/g)];
    if (path.endsWith('.astro')) {
      const frontmatter = /^---\r?\n[\s\S]*?\r?\n---/.exec(content);
      if (imports(content.slice(frontmatter?.[0].length ?? 0)).length) throw new Error(`Server helper imported by browser script: ${path}`);
    } else if (!/\.server\.[cm]?[jt]s$/.test(path) && imports(content).length) {
      throw new Error(`Server helper imported by browser module: ${path}`);
    }
    if (path.startsWith('apps/web/src/pages/resources/') && /\bclient:[\w-]+/.test(content)) throw new Error(`Resource hydration is forbidden: ${path}`);
  }
  if (sinks !== 1) throw new Error('Expected exactly one audited Markdown HTML sink');
}
