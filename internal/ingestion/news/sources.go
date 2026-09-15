package news

import "github.com/shopspring/decimal"

type Source struct {
	Name        string
	URL         string
	Type        string
	Channel     string
	TrustWeight decimal.Decimal
}

func DefaultSources() []Source {
	return []Source{
		{Name: "РБК", URL: "https://rssexport.rbc.ru/rbcnews/news/30/full.rss", TrustWeight: decimal.NewFromFloat(1.0)},
		{Name: "Интерфакс", URL: "https://www.interfax.ru/rss", TrustWeight: decimal.NewFromFloat(0.85)},
		{Name: "Коммерсантъ", URL: "https://www.kommersant.ru/rss/news.xml", TrustWeight: decimal.NewFromFloat(0.95)},
		{Name: "Ведомости", URL: "https://www.vedomosti.ru/rss/news", TrustWeight: decimal.NewFromFloat(1.0)},
		{Name: "Prime", URL: "https://1prime.ru/export/rss2/index.xml", TrustWeight: decimal.NewFromFloat(1.0)},
		{Name: "Smart-lab", URL: "https://smart-lab.ru/news/rss", TrustWeight: decimal.NewFromFloat(0.65)},
		{Name: "Finam", URL: "https://www.finam.ru/analysis/conews/rsspoint/", TrustWeight: decimal.NewFromFloat(0.8)},
		{Name: "Frank Media", URL: "https://frankmedia.ru/feed", TrustWeight: decimal.NewFromFloat(0.95)},
		{Name: "Ведомости.Бизнес", URL: "https://www.vedomosti.ru/rss/rubric/business", TrustWeight: decimal.NewFromFloat(1.0)},
		{Name: "Ведомости.Финансы", URL: "https://www.vedomosti.ru/rss/rubric/finance", TrustWeight: decimal.NewFromFloat(1.0)},
		{Name: "Коммерсантъ.Финансы", URL: "https://www.kommersant.ru/rss/section-finance.xml", TrustWeight: decimal.NewFromFloat(0.7)},
		{Name: "Финмаркет", URL: "https://www.finmarket.ru/rss/mainnews.asp", TrustWeight: decimal.NewFromFloat(1.0)},
		{Name: "MarketTwits (TG)", Type: "telegram", Channel: "markettwits", TrustWeight: decimal.NewFromFloat(0.25)},
		{Name: "Банкста (TG)", Type: "telegram", Channel: "banksta", TrustWeight: decimal.NewFromFloat(0.1)},
	}
}

func DefaultAliases() map[string][]string {
	return map[string][]string{
		"YDEX":       {"яндекс", "яндекса", "яндексу", "яндексом", "yandex", "ydex"},
		"OZON":       {"озон", "озона", "озону", "ozon"},
		"SBER":       {"сбербанк", "сбербанка", "сбер", "сбера", "sberbank", "SBER"},
		"LKOH":       {"лукойл", "лукойла", "lukoil", "LKOH"},
		"GAZP":       {"газпром", "газпрома", "gazprom", "GAZP"},
		"GMKN":       {"норникель", "норникеля", "норильский никель", "nornickel", "GMKN"},
		"ROSN":       {"роснефть", "роснефти", "rosneft", "ROSN"},
		"NVTK":       {"новатэк", "новатэка", "novatek", "NVTK"},
		"TATN":       {"татнефть", "татнефти", "tatneft", "TATN"},
		"MTSS":       {"мтс", "mts", "MTSS"},
		"MGNT":       {"магнит", "магнита", "magnit", "MGNT"},
		"PLZL":       {"полюса", "polyus", "PLZL"},
		"CHMF":       {"северсталь", "северстали", "severstal", "CHMF"},
		"DATA":       {"аренадата", "аренадаты", "arenadata"},
		"T":          {"т-технологии", "т-технологий", "тинькофф", "тинькоффа", "т-банк", "тбанк", "tinkoff", "t-technologies"},
		"SBMM":       {"первая сберегательный", "sbmm"},
		"VTBR":       {"втб", "vtb", "VTBR"},
		"RUAL":       {"русал", "русала", "rusal", "RUAL"},
		"GLDRUB_TOM": {"золото", "золота", "золоту", "золотом", "gold", "gldrub"},
		"SLVRUB_TOM": {"серебро", "серебра", "серебру", "серебром", "silver", "slvrub"},
		"CNYRUB_TOM": {"юань", "юаня", "юаню", "юанем", "юани", "юаней", "cnyrub", "renminbi"},
	}
}
