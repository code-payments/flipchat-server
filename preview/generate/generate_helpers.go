package generate

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/code-payments/flipchat-server/image"
	"golang.org/x/net/html"

	commonpb "github.com/code-payments/flipchat-protobuf-api/generated/go/common/v1"
)

// htmlParse wraps the reader in a LimitedReader (for safety) and parses the HTML document.
func htmlParse(body io.Reader) (*html.Node, error) {
	// Limit the reader to 1MB to avoid huge downloads
	limitedReader := io.LimitReader(body, 1*1024*1024)
	doc, err := html.Parse(limitedReader)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// extractTitle tries multiple methods to get a meaningful title.
func extractTitle(doc *html.Node, baseURL string) string {
	// Try <title>
	title := findTextByTag(doc, "title")
	if title != "" {
		return title
	}
	// Fallback to first <h1> or <h2>
	title = findTextByTag(doc, "h1")
	if title == "" {
		title = findTextByTag(doc, "h2")
	}
	// If still empty, fallback to domain name
	if title == "" {
		if u, err := url.Parse(baseURL); err == nil {
			title = u.Hostname()
		}
	}
	return title
}

// extractDescription tries meta description, then meta og:description, then first <p> tag.
func extractDescription(doc *html.Node) string {
	// Try meta tags: description and og:description
	if content := findMetaContent(doc, "name", "description"); content != "" {
		return content
	}
	if content := findMetaContent(doc, "property", "og:description"); content != "" {
		return content
	}
	// Fallback to first <p> tag
	return findTextByTag(doc, "p")
}

// extractMainImageURL checks for meta og:image first, then falls back to the first <img> tag.
func extractMainImageURL(doc *html.Node, baseURL string) string {
	// Check for meta og:image or twitter:image
	if imgURL := findMetaContent(doc, "property", "og:image"); imgURL != "" {
		if resolved, err := resolveURL(imgURL, baseURL); err == nil {
			return resolved
		}
	}
	if imgURL := findMetaContent(doc, "name", "twitter:image"); imgURL != "" {
		if resolved, err := resolveURL(imgURL, baseURL); err == nil {
			return resolved
		}
	}
	// Fallback: first <img> tag
	return findFirstImg(doc, baseURL)
}

// findTextByTag traverses the document tree looking for the first occurrence of a tag.
func findTextByTag(n *html.Node, tag string) string {
	if n.Type == html.ElementNode && n.Data == tag {
		if n.FirstChild != nil {
			return strings.TrimSpace(n.FirstChild.Data)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if text := findTextByTag(c, tag); text != "" {
			return text
		}
	}
	return ""
}

// findMetaContent returns the content attribute of a meta tag matching the given key and value.
func findMetaContent(n *html.Node, key, val string) string {
	if n.Type == html.ElementNode && n.Data == "meta" {
		var attrKey, content string
		for _, attr := range n.Attr {
			lowerKey := strings.ToLower(attr.Key)
			switch lowerKey {
			case key:
				if strings.ToLower(attr.Val) == strings.ToLower(val) {
					attrKey = attr.Val
				}
			case "content":
				content = attr.Val
			}
		}
		if attrKey != "" && content != "" {
			return content
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if content := findMetaContent(c, key, val); content != "" {
			return content
		}
	}
	return ""
}

// findFirstImg finds the first <img> tag and resolves its src attribute.
func findFirstImg(n *html.Node, baseURL string) string {
	if n.Type == html.ElementNode && n.Data == "img" {
		for _, attr := range n.Attr {
			if strings.ToLower(attr.Key) == "src" {
				if resolved, err := resolveURL(attr.Val, baseURL); err == nil {
					return resolved
				}
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if src := findFirstImg(c, baseURL); src != "" {
			return src
		}
	}
	return ""
}

// resolveURL resolves a potentially relative URL against a base URL.
func resolveURL(href, base string) (string, error) {
	parsedBase, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	parsedHref, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	return parsedBase.ResolveReference(parsedHref).String(), nil
}

// parseHTML extracts the title, description, and main image URL from the HTML content.
func parseHTML(body io.Reader, baseURL string) (title, description, imageURL string, err error) {
	// Parse HTML safely with a limited reader.
	doc, err := htmlParse(body)
	if err != nil {
		return "", "", "", err
	}

	// Use enhanced extraction functions with fallbacks.
	title = extractTitle(doc, baseURL)
	description = extractDescription(doc)
	imageURL = extractMainImageURL(doc, baseURL)

	return title, description, imageURL, nil
}

// fetchAndProcessImage retrieves the image from imageURL and processes it.
func fetchAndProcessImage(ctx context.Context, imageURL string) (*commonpb.ImageInfo, error) {
	if imageURL == "" {
		return &commonpb.ImageInfo{}, nil
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return nil, err
	}

	// Set User-Agent.
	req.Header.Set("User-Agent", "FlipchatPreviewBot/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check for non-200 status codes.
	if resp.StatusCode != http.StatusOK {
		return nil, ErrFetchURL
	}

	// Read image data.
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Process the image to get dimensions and BlurHash.
	imageInfo, err := image.ProcessImage(imageData)
	if err != nil {
		return nil, err
	}

	return imageInfo, nil
}

// detectContentType inspects the Content-Type header and returns a corresponding enum value.
func detectContentType(header string) commonpb.ContentType {
	if header == "" {
		return commonpb.ContentType_CONTENT_TYPE_UNKNOWN
	}
	if strings.Contains(header, "text/html") {
		return commonpb.ContentType_CONTENT_TYPE_TEXT
	} else if strings.HasPrefix(header, "image/") {
		return commonpb.ContentType_CONTENT_TYPE_IMAGE
	} else if strings.HasPrefix(header, "video/") {
		return commonpb.ContentType_CONTENT_TYPE_VIDEO
	} else if strings.HasPrefix(header, "audio/") {
		return commonpb.ContentType_CONTENT_TYPE_AUDIO
	} else if strings.Contains(header, "application/pdf") {
		return commonpb.ContentType_CONTENT_TYPE_PDF
	}
	return commonpb.ContentType_CONTENT_TYPE_FILE
}

// extractFileNameFromURL extracts the file name (or last path segment) from the URL.
func extractFileNameFromURL(urlStr string) string {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return urlStr // Fallback to the full URL if parsing fails.
	}
	segments := strings.Split(parsed.Path, "/")
	if len(segments) > 0 {
		fileName := segments[len(segments)-1]
		if fileName != "" {
			return fileName
		}
	}
	return parsed.Hostname()
}
