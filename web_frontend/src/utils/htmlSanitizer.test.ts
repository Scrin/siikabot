import { describe, it, expect } from 'vitest'
import { sanitizeHtml } from './htmlSanitizer'

describe('sanitizeHtml', () => {
  describe('basic sanitization', () => {
    it('should return empty string for null/undefined', () => {
      expect(sanitizeHtml(null as unknown as string)).toBe('')
      expect(sanitizeHtml(undefined as unknown as string)).toBe('')
      expect(sanitizeHtml('')).toBe('')
    })

    it('should preserve allowed elements', () => {
      expect(sanitizeHtml('<b>bold</b>')).toBe('<b>bold</b>')
      expect(sanitizeHtml('<i>italic</i>')).toBe('<i>italic</i>')
      expect(sanitizeHtml('<span>text</span>')).toBe('<span>text</span>')
    })

    it('should preserve table structure', () => {
      const html =
        '<table><thead><tr><th>Header</th></tr></thead><tbody><tr><td>Cell</td></tr></tbody></table>'
      expect(sanitizeHtml(html)).toBe(html)
    })

    it('should strip disallowed elements but keep text', () => {
      expect(sanitizeHtml('<div>content</div>')).toBe('content')
      expect(sanitizeHtml('<p>paragraph</p>')).toBe('paragraph')
    })

    it('should strip script tags completely (browser behavior)', () => {
      const result = sanitizeHtml('<script>alert("xss")</script>')
      // Script content is not accessible via DOM parsing - browser strips it completely
      expect(result).toBe('')
    })

    it('should handle nested disallowed elements', () => {
      expect(sanitizeHtml('<div><p><span>text</span></p></div>')).toBe('<span>text</span>')
    })
  })

  describe('attribute sanitization', () => {
    it('should preserve allowed attributes on anchor tags', () => {
      const html = '<a href="https://example.com" title="Link">text</a>'
      expect(sanitizeHtml(html)).toBe(html)
    })

    it('should strip disallowed attributes', () => {
      const html = '<a href="https://example.com" onclick="alert(1)">text</a>'
      expect(sanitizeHtml(html)).toBe('<a href="https://example.com">text</a>')
    })

    it('should preserve colspan/rowspan on td/th within tables', () => {
      // td/th elements are only valid within table structure
      // Browser automatically adds tbody when parsing, so we check for content
      const tdHtml = '<table><tr><td colspan="2">cell</td></tr></table>'
      const tdResult = sanitizeHtml(tdHtml)
      expect(tdResult).toContain('<td colspan="2">cell</td>')
      expect(tdResult).toContain('<table>')

      const thHtml = '<table><tr><th rowspan="3">header</th></tr></table>'
      const thResult = sanitizeHtml(thHtml)
      expect(thResult).toContain('<th rowspan="3">header</th>')
      expect(thResult).toContain('<table>')
    })

    it('should strip attributes from elements without allowed attributes', () => {
      expect(sanitizeHtml('<b class="bold" style="color:red">text</b>')).toBe('<b>text</b>')
    })
  })

  describe('XSS prevention', () => {
    it('should strip javascript: URLs', () => {
      const html = '<a href="javascript:alert(1)">click</a>'
      expect(sanitizeHtml(html)).toBe('<a>click</a>')
    })

    it('should strip data: URLs', () => {
      const html = '<a href="data:text/html,<script>alert(1)</script>">click</a>'
      expect(sanitizeHtml(html)).toBe('<a>click</a>')
    })

    it('should allow http URLs', () => {
      expect(sanitizeHtml('<a href="http://example.com">link</a>')).toBe(
        '<a href="http://example.com">link</a>'
      )
    })

    it('should allow https URLs', () => {
      expect(sanitizeHtml('<a href="https://example.com">link</a>')).toBe(
        '<a href="https://example.com">link</a>'
      )
    })

    it('should allow mailto: URLs', () => {
      const html = '<a href="mailto:user@example.com">email</a>'
      expect(sanitizeHtml(html)).toBe(html)
    })

    it('should allow relative URLs starting with /', () => {
      expect(sanitizeHtml('<a href="/path">link</a>')).toBe('<a href="/path">link</a>')
    })

    it('should allow relative URLs starting with ./', () => {
      expect(sanitizeHtml('<a href="./path">link</a>')).toBe('<a href="./path">link</a>')
    })

    it('should allow relative URLs starting with ../', () => {
      expect(sanitizeHtml('<a href="../path">link</a>')).toBe('<a href="../path">link</a>')
    })

    it('should strip event handlers', () => {
      const html = '<span onmouseover="alert(1)">text</span>'
      expect(sanitizeHtml(html)).toBe('<span>text</span>')
    })

    it('should strip onerror handlers', () => {
      const html = '<span onerror="alert(1)">text</span>'
      expect(sanitizeHtml(html)).toBe('<span>text</span>')
    })
  })

  describe('edge cases', () => {
    it('should handle deeply nested structures', () => {
      const html =
        '<table><tbody><tr><td><b><i><span>deep</span></i></b></td></tr></tbody></table>'
      expect(sanitizeHtml(html)).toBe(html)
    })

    it('should handle text nodes at root level', () => {
      expect(sanitizeHtml('plain text')).toBe('plain text')
    })

    it('should handle mixed content', () => {
      expect(sanitizeHtml('text <b>bold</b> more text')).toBe('text <b>bold</b> more text')
    })

    it('should handle empty elements', () => {
      expect(sanitizeHtml('<b></b>')).toBe('<b></b>')
      expect(sanitizeHtml('<table></table>')).toBe('<table></table>')
    })

    it('should handle multiple sibling elements', () => {
      expect(sanitizeHtml('<b>one</b><i>two</i><span>three</span>')).toBe(
        '<b>one</b><i>two</i><span>three</span>'
      )
    })

    it('should handle whitespace preservation', () => {
      expect(sanitizeHtml('<b>  spaces  </b>')).toBe('<b>  spaces  </b>')
    })

    it('should handle complex table with multiple rows', () => {
      const html = `<table>
        <thead><tr><th>Col 1</th><th>Col 2</th></tr></thead>
        <tbody>
          <tr><td>A</td><td>B</td></tr>
          <tr><td>C</td><td>D</td></tr>
        </tbody>
      </table>`
      const result = sanitizeHtml(html)
      expect(result).toContain('<thead>')
      expect(result).toContain('<tbody>')
      expect(result).toContain('<th>Col 1</th>')
      expect(result).toContain('<td>A</td>')
    })
  })
})
