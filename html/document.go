package html

import (
	"code.google.com/p/go.net/html"
	"errors"
	"github.com/slyrz/newscat/util"
	"io"
	"net/url"
	"unicode"
)

const (
	chunkCap = 512 // initial capacity of the Article.Chunks array
	linkCap  = 256 // initial capacity of the Website.Links array
	feedCap  = 4   // initial capacity of the Website.Feeds array
)

const (
	// We remember a few special node types when descending into their
	// children.
	AncestorArticle = 1 << iota
	AncestorAside
	AncestorBlockquote
	AncestorList
)

// Document is a parsed HTML document that extracts the document title and
// holds unexported pointers to the html, head and body nodes.
type Document struct {
	Title *util.Text // the <title>...</title> text.

	// Unexported fields.
	html *html.Node // the <html>...</html> part
	head *html.Node // the <head>...</head> part
	body *html.Node // the <body>...</body> part
}

// Article stores all text chunks found in a HTML document.
type Article struct {
	Document
	Chunks []*Chunk // all chunks found in this document.

	// State variables used when collectiong chunks.
	ancestors int // bitmask which stores ancestor types of the current node

	// Number of non-space characters inside link tags / normal tags
	// per html.ElementNode.
	linkText map[*html.Node]int // length of text inside <a></a> tags
	normText map[*html.Node]int // length of text outside <a></a> tags
}

// Website finds all links in a HTML document.
type Website struct {
	Document
	Links []*Link // all links found in this document.
	Feeds []*Link // all RSS feeds found in this document.
}

// NewDocument parses the HTML data provided through an io.Reader interface.
func NewDocument(r io.Reader) (*Document, error) {
	doc := new(Document)
	if err := doc.init(r); err != nil {
		return nil, err
	}
	return doc, nil
}

func (doc *Document) init(r io.Reader) error {
	doc.Title = util.NewText()

	root, err := html.Parse(r)
	if err != nil {
		return err
	}

	// Assign the fields html, head and body from the HTML page.
	iterateNode(root, func(n *html.Node) int {
		switch n.Data {
		case "html":
			doc.html = n
			return IterNext
		case "body":
			doc.body = n
			return IterSkip
		case "head":
			doc.head = n
			return IterSkip
		}
		// Keep going as long as we're missing some nodes.
		return IterNext
	})

	// Check if html, head and body nodes were found.
	if doc.html == nil || doc.head == nil || doc.body == nil {
		return errors.New("Document missing <html>, <head> or <body>.")
	}

	// Detect the document title.
	iterateNode(doc.head, func(n *html.Node) int {
		if n.Type == html.ElementNode && n.Data == "title" {
			iterateText(n, doc.Title.WriteString)
			return IterStop
		}
		return IterNext
	})
	return nil
}

// NewWebsite parses the HTML data provided through an io.Reader interface
// and returns, if successful, a Website object that can be used to access
// all links and extract links to news articles.
func NewWebsite(r io.Reader) (*Website, error) {
	website := new(Website)
	if err := website.init(r); err != nil {
		return nil, err
	}
	return website, nil
}

const (
	linkRelAlternate = 1 << iota
	linkTypeRss
)

func (website *Website) init(r io.Reader) error {
	if err := website.Document.init(r); err != nil {
		return err
	}
	website.Links = make([]*Link, 0, linkCap)
	website.Feeds = make([]*Link, 0, feedCap)

	// Extract all links.
	iterateNode(website.body, func(n *html.Node) int {
		if n.Type == html.ElementNode && n.Data == "a" {
			if link, err := NewLink(n); err == nil {
				website.Links = append(website.Links, link)
			}
			return IterSkip
		}
		return IterNext
	})

	// Extract all RSS feeds.
	iterateNode(website.head, func(n *html.Node) int {
		if n.Data != "link" {
			return IterNext
		}
		// Scan the link attributes and make sure we find
		//	rel="alternate" type="application/rss+xml" href="..."
		href, hasAttr := "", 0
		for _, attr := range n.Attr {
			switch {
			case attr.Key == "rel" && attr.Val == "alternate":
				hasAttr |= linkRelAlternate
			case attr.Key == "type" && attr.Val == "application/rss+xml":
				hasAttr |= linkTypeRss
			case attr.Key == "href":
				href = attr.Val
			}
		}
		if hasAttr != (linkTypeRss|linkRelAlternate) || href == "" {
			return IterNext
		}
		if link, err := NewLinkFromString(href); err == nil {
			website.Feeds = append(website.Feeds, link)
		}
		return IterNext
	})
	return nil
}

// ResolveBase transforms relative URLs to absolute URLs by adding
// missing components from an absolute base URL.
func (website *Website) ResolveBase(base string) error {
	baseURL, err := url.Parse(base)
	if err == nil {
		for _, link := range website.Links {
			link.Resolve(baseURL)
		}
		for _, feed := range website.Feeds {
			feed.Resolve(baseURL)
		}
	}
	return err
}

// NewArticle parses the HTML data provided through an io.Reader interface
// and returns, if successful, an Article object that can be used to access
// all relevant text chunks found in the document.
func NewArticle(r io.Reader) (*Article, error) {
	article := new(Article)
	if err := article.init(r); err != nil {
		return nil, err
	}
	return article, nil
}

func (article *Article) init(r io.Reader) error {
	if err := article.Document.init(r); err != nil {
		return err
	}

	article.Chunks = make([]*Chunk, 0, chunkCap)
	article.linkText = make(map[*html.Node]int)
	article.normText = make(map[*html.Node]int)

	article.cleanBody(article.body, 0)
	article.countText(article.body, false)
	article.parseBody(article.body)

	// Now we link the chunks.
	min, max := 0, len(article.Chunks)-1
	for i := range article.Chunks {
		if i > min {
			article.Chunks[i].Prev = article.Chunks[i-1]
		}
		if i < max {
			article.Chunks[i].Next = article.Chunks[i+1]
		}
	}
	return nil
}

// countText counts the text inside of links and the text outside of links
// per html.Node. Counting is done cumulative, so the umbers of a parent node
// include the numbers of it's child nodes.
func (article *Article) countText(n *html.Node, insideLink bool) (linkText int, normText int) {
	linkText = 0
	normText = 0
	if n.Type == html.ElementNode && n.Data == "a" {
		insideLink = true
	}
	for s := n.FirstChild; s != nil; s = s.NextSibling {
		linkTextChild, normTextChild := article.countText(s, insideLink)
		linkText += linkTextChild
		normText += normTextChild
	}
	if n.Type == html.TextNode {
		count := 0
		for _, rune := range n.Data {
			if unicode.IsLetter(rune) {
				count += 1
			}
		}
		if insideLink {
			linkText += count
		} else {
			normText += count
		}
	}
	article.linkText[n] = linkText
	article.normText[n] = normText
	return
}

// cleanBody removes unwanted HTML elements from the HTML body.
func (article *Article) cleanBody(n *html.Node, level int) {

	// removeNode returns true if a node should be removed from HTML document.
	removeNode := func(c *html.Node, level int) bool {
		switch c.Data {
		// Elements save to ignore.
		case "address", "audio", "button", "canvas", "caption", "fieldset",
			"figcaption", "figure", "footer", "form", "frame", "iframe",
			"map", "menu", "nav", "noscript", "object", "option", "output",
			"script", "select", "style", "svg", "textarea", "video":
			return true
		// High-level tables might be used to layout the document, so we better
		// not ignore them.
		case "table":
			return level > 5
		}
		return false
	}

	var curr *html.Node = n.FirstChild
	var next *html.Node = nil
	for ; curr != nil; curr = next {
		// We have to remember the next sibling here because calling RemoveChild
		// sets curr's NextSibling pointer to nil and we would quit the loop
		// prematurely.
		next = curr.NextSibling
		if curr.Type == html.ElementNode {
			if removeNode(curr, level) {
				n.RemoveChild(curr)
			} else {
				article.cleanBody(curr, level+1)
			}
		}
	}
}

var (
	ignoreNames = util.NewRegexFromWords(
		"breadcrumb",
		"byline",
		"caption",
		"comment",
		"community",
		"credit",
		"description",
		"email",
		"foot",
		"gallery",
		"hide",
		"infotext",
		"photo",
		"related",
		"shares",
		"social",
		"story[-_]?bar",
		"story[-_]?feature",
	)
	ignoreStyle = util.NewRegex(`(?i)display:\s*none`)
)

// parseBody parses the <body>...</body> part of the HTML page. It creates
// Chunks for every html.TextNode found in the body.
func (article *Article) parseBody(n *html.Node) {
	switch n.Type {
	case html.ElementNode:
		// We ignore the node if it has some nasty classes/ids/itemprops or if
		// its style attribute contains "display: none".
		for _, attr := range n.Attr {
			switch attr.Key {
			case "id", "class", "itemprop":
				if ignoreNames.In(attr.Val) {
					return
				}
			case "style":
				if ignoreStyle.In(attr.Val) {
					return
				}
			}
		}
		ancestorMask := 0
		switch n.Data {
		// We convert headings and links to text immediately. This is easier
		// and feasible because headings and links don't contain many children.
		// Descending into these children and handling every TextNode separately
		// would make things unnecessary complicated and our results noisy.
		case "h1", "h2", "h3", "h4", "h5", "h6", "a":
			if chunk, err := NewChunk(article, n); err == nil {
				article.Chunks = append(article.Chunks, chunk)
			}
			return
		// Now mask the element type, but only if it isn't already set.
		// If we mask a bit which was already set by one of our callers, we'd also
		// clear it at the end of this function, though it actually should be cleared
		// by the caller.
		case "article":
			ancestorMask = AncestorArticle &^ article.ancestors
		case "aside":
			ancestorMask = AncestorAside &^ article.ancestors
		case "blockquote":
			ancestorMask = AncestorBlockquote &^ article.ancestors
		case "ul", "ol":
			ancestorMask = AncestorList &^ article.ancestors
		}
		// Add our mask to the ancestor bitmask.
		article.ancestors |= ancestorMask
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			article.parseBody(c)
		}
		// Remove our mask from the ancestor bitmask.
		article.ancestors &^= ancestorMask
	case html.TextNode:
		if chunk, err := NewChunk(article, n); err == nil {
			article.Chunks = append(article.Chunks, chunk)
		}
	}
}

// TextStat contains the number of words and sentences found in text.
type TextStat struct {
	Words     int // total number of words
	Sentences int // total number of sentences
	Count     int // number of texts used to calculate this stats
}

// GetClassStats groups the document chunks by their classes (defined by the
// class attribute of HTML nodes) and calculates TextStats for each class.
func (article *Article) GetClassStats() map[string]*TextStat {
	result := make(map[string]*TextStat)
	for _, chunk := range article.Chunks {
		for _, class := range chunk.Classes {
			if stat, ok := result[class]; ok {
				stat.Words += chunk.Text.Words
				stat.Sentences += chunk.Text.Sentences
				stat.Count += 1
			} else {
				result[class] = &TextStat{chunk.Text.Words, chunk.Text.Sentences, 1}
			}
		}
	}
	return result
}

// GetClusterStats groups the document chunks by common ancestors and
// calculates TextStats for each group of chunks.
func (article *Article) GetClusterStats() map[*Chunk]*TextStat {
	// Don't ascend further than this.
	const maxAncestors = 3

	// Count TextStats for Chunk ancestors.
	ancestorStat := make(map[*html.Node]*TextStat)
	for _, chunk := range article.Chunks {
		node, count := chunk.Block, 0
		for node != nil && count < maxAncestors {
			if stat, ok := ancestorStat[node]; ok {
				stat.Words += chunk.Text.Words
				stat.Sentences += chunk.Text.Sentences
				stat.Count += 1
			} else {
				ancestorStat[node] = &TextStat{chunk.Text.Words, chunk.Text.Sentences, 1}
			}
			node, count = node.Parent, count+1
		}
	}

	// Generate result.
	result := make(map[*Chunk]*TextStat)
	for _, chunk := range article.Chunks {
		node := chunk.Block
		if node == nil {
			continue
		}
		// Start with the parent's TextStat. Then ascend and check if the
		// current chunk has an ancestor with better stats. Use the best stat
		// as result.
		stat := ancestorStat[node]
		for {
			if node = node.Parent; node == nil {
				break
			}
			if statPrev, ok := ancestorStat[node]; ok {
				if stat.Count < statPrev.Count {
					stat = statPrev
				}
			} else {
				break
			}
		}
		result[chunk] = stat
	}
	return result
}
