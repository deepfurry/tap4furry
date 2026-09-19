import MarkdownIt from 'markdown-it';
import sanitizeHtml from 'sanitize-html';

const parser = new MarkdownIt({ html: false, linkify: false, typographer: false });
parser.renderer.rules.image = () => '';
for (const rule of ['heading_open', 'heading_close']) {
  parser.renderer.rules[rule] = (tokens, index, options, _env, renderer) => {
    if (tokens[index].tag === 'h1') tokens[index].tag = 'h2';
    return renderer.renderToken(tokens, index, options);
  };
}

// Only Markdown.astro may consume this HTML. No plugins, raw HTML, images or MDX.
export function renderMarkdown(source: string): string {
  return sanitizeHtml(parser.render(source), {
    allowedTags: ['p', 'h2', 'h3', 'h4', 'h5', 'h6', 'strong', 'em', 's', 'del', 'ul', 'ol', 'li', 'blockquote', 'code', 'pre', 'a', 'hr', 'br', 'table', 'thead', 'tbody', 'tr', 'th', 'td'],
    allowedAttributes: { a: ['href', 'rel'], ol: ['start'] },
    allowedSchemes: ['http', 'https'],
    allowProtocolRelative: false,
    transformTags: { a: (_tag, attributes) => ({ tagName: 'a', attribs: { href: attributes.href ?? '', rel: 'ugc nofollow noreferrer' } }) },
  });
}
