// HTML Sanitizer for safe rendering of Grafana template output
// Only allows specific elements that are safe and supported by Matrix

const ALLOWED_ELEMENTS = new Set(['table', 'tr', 'th', 'td', 'b', 'i', 'a', 'span', 'tbody', 'thead'])

const ALLOWED_ATTRIBUTES: Record<string, Set<string>> = {
  a: new Set(['href', 'title']),
  td: new Set(['colspan', 'rowspan']),
  th: new Set(['colspan', 'rowspan']),
}

const ALLOWED_PROTOCOLS = ['http:', 'https:', 'mailto:']

/**
 * Sanitizes HTML string to only allow safe elements and attributes.
 * Prevents XSS by stripping scripts, event handlers, and dangerous protocols.
 */
export function sanitizeHtml(html: string): string {
  if (!html || typeof html !== 'string') {
    return ''
  }

  const parser = new DOMParser()
  const doc = parser.parseFromString(html, 'text/html')

  const sanitizedFragment = document.createDocumentFragment()
  sanitizeNode(doc.body, sanitizedFragment)

  const container = document.createElement('div')
  container.appendChild(sanitizedFragment)
  return container.innerHTML
}

function sanitizeNode(source: Node, target: Node): void {
  for (const child of Array.from(source.childNodes)) {
    if (child.nodeType === Node.TEXT_NODE) {
      target.appendChild(document.createTextNode(child.textContent || ''))
    } else if (child.nodeType === Node.ELEMENT_NODE) {
      const element = child as Element
      const tagName = element.tagName.toLowerCase()

      if (ALLOWED_ELEMENTS.has(tagName)) {
        const sanitizedElement = document.createElement(tagName)

        // Copy only allowed attributes
        const allowedAttrs = ALLOWED_ATTRIBUTES[tagName]
        if (allowedAttrs) {
          for (const attr of Array.from(element.attributes)) {
            if (allowedAttrs.has(attr.name.toLowerCase())) {
              // Special handling for href to prevent javascript: URLs
              if (attr.name.toLowerCase() === 'href') {
                const sanitizedHref = sanitizeHref(attr.value)
                if (sanitizedHref) {
                  sanitizedElement.setAttribute(attr.name, sanitizedHref)
                }
              } else {
                sanitizedElement.setAttribute(attr.name, attr.value)
              }
            }
          }
        }

        target.appendChild(sanitizedElement)
        sanitizeNode(element, sanitizedElement)
      } else {
        // Element not allowed - include its text content only
        sanitizeNode(element, target)
      }
    }
  }
}

function sanitizeHref(href: string): string | null {
  try {
    const url = new URL(href, window.location.origin)
    if (ALLOWED_PROTOCOLS.includes(url.protocol)) {
      return href
    }
  } catch {
    // Invalid URL - check if it's a relative path
    if (href.startsWith('/') || href.startsWith('./') || href.startsWith('../')) {
      return href
    }
  }
  return null
}
