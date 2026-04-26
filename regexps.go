package readability

import "regexp"

var (
	unlikelyCandidateRE = regexp.MustCompile(`(?i)-ad-|ai2html|banner|breadcrumbs|combx|comment|community|cover-wrap|disqus|extra|footer|gdpr|header|legends|menu|related|remark|replies|rss|shoutbox|sidebar|skyscraper|social|sponsor|supplemental|ad-break|agegate|pagination|pager|popup|yom-remote`)
	okMaybeCandidateRE  = regexp.MustCompile(`(?i)and|article|body|column|content|main|mathjax|shadow`)
	positiveCandidateRE = regexp.MustCompile(`(?i)article|body|content|entry|hentry|h-entry|main|page|pagination|post|text|blog|story`)
	negativeCandidateRE = regexp.MustCompile(`(?i)-ad-|hidden|^hid$| hid$| hid |^hid |banner|combx|comment|com-|contact|footer|gdpr|masthead|media|meta|outbrain|promo|related|scroll|share|shoutbox|sidebar|skyscraper|sponsor|shopping|tags|widget`)
	videoURLRE          = regexp.MustCompile(`(?i)//(www\.)?((dailymotion|youtube|youtube-nocookie|player\.vimeo|v\.qq|bilibili|live\.bilibili)\.com|(archive|upload\.wikimedia)\.org|player\.twitch\.tv)`)
	adWordsRE           = regexp.MustCompile(`(?i)^(ad(vertising|vertisement)?|pub(licité)?|werb(ung)?|广告|Реклама|Anuncio)$`)
	loadingWordsRE      = regexp.MustCompile(`(?i)^((loading|正在加载|Загрузка|chargement|cargando)(…|\\.\\.\\.)?)$`)
	b64DataURLRE        = regexp.MustCompile(`(?i)^data:\s*([^\s;,]+)\s*;\s*base64\s*,`)
	imageURLRE          = regexp.MustCompile(`(?i)\.(jpg|jpeg|png|webp)(\s+\d|\?|#|$)`)
	imageSrcsetRE       = regexp.MustCompile(`(?i)\.(jpg|jpeg|png|webp)\s+\d`)
	srcsetURLRE         = regexp.MustCompile(`(\S+)(\s+[\d.]+[xw])?(\s*(?:,|$))`)
	leadingDateRE       = regexp.MustCompile(`^\d{1,2}[./]\d{1,2}[./]\d{2,4}\s+\d{1,2}:\d{2}$`)
)
