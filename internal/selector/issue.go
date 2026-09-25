package selector

// defaultIssue 返回期号页的定稿 selector（§8.1(1)）。
// 注意：禁止对整页文本全扫期号——期号页同时存在“总第1224期”与“2026年第37期”，
// 全页扫描会把卷期 37 误判为期号，故期号正则只允许作用在 I1/I2/I3 三个节点上。
func defaultIssue() Issue {
	return Issue{
		PeriodTitle:       "div.mainMagContent div.report div.title",
		PeriodHeadTitle:   "head > title",
		PeriodBreadcrumb:  "div.positionNav > a",
		VolumeSource:      "div.source",
		VolumeDate:        "div.date span",
		VolumeArticleInfo: "div#artInfo",
		EntryContainers:   []string{"div.report dl", "div.magContent2 dl"},
		EntryLink:         "dl > dt > a[href]",
		EntryTitle:        "dl > dt > a",
		EntryByline:       "dl > dd.date",
		EntrySummary:      "dl > dd",
		ColumnName:        "div.magIntrotit > span",
		ExcludeLinkScopes: []string{
			"div.cover div.subscribe a",
			"div.positionNav a",
			"div.bottom",
			"div.navBottom",
		},
	}
}
