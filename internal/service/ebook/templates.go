package ebook

// modifiedTimestamp 是 EPUB 3 必需的 dcterms:modified；固定值保证构建可复现。
const modifiedTimestamp = "2026-01-01T00:00:00Z"

// containerDocument 是 EPUB 固定入口，指向包文档。
const containerDocument = `<?xml version="1.0" encoding="utf-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
`

// packageTemplate 是 OEBPS/content.opf：依次填入 identifier、书名、修改时间、清单与阅读顺序。
const packageTemplate = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="bookid">urn:caixin2kindle:%s</dc:identifier>
    <dc:title>%s</dc:title>
    <dc:language>zh-CN</dc:language>
    <dc:creator>财新周刊</dc:creator>
    <meta property="dcterms:modified">%s</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
%s  </manifest>
  <spine>
%s  </spine>
</package>
`

// navigationTemplate 是 OEBPS/nav.xhtml：填入按目录顺序排列的章节链接。
const navigationTemplate = `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="zh-CN">
  <head>
    <meta charset="utf-8"/>
    <title>目录</title>
  </head>
  <body>
    <nav epub:type="toc" id="toc">
      <h1>目录</h1>
      <ol>
%s      </ol>
    </nav>
  </body>
</html>
`

// chapterTemplate 是单章 XHTML：依次填入 <title>、<h1>、署名与段落。
const chapterTemplate = `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" lang="zh-CN">
  <head>
    <meta charset="utf-8"/>
    <title>%s</title>
  </head>
  <body>
    <section>
      <h1>%s</h1>
%s%s    </section>
  </body>
</html>
`
