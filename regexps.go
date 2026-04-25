package readability

import "regexp"

var (
	unlikelyCandidateRE = regexp.MustCompile(`(?i)-ad-|ai2html|banner|breadcrumbs|combx|comment|community|cover-wrap|disqus|extra|footer|gdpr|header|legends|menu|related|remark|replies|rss|shoutbox|sidebar|skyscraper|social|sponsor|supplemental|ad-break|agegate|pagination|pager|popup|yom-remote`)
	okMaybeCandidateRE  = regexp.MustCompile(`(?i)and|article|body|column|content|main|mathjax|shadow`)
)
