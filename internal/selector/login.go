package selector

// defaultLogin 返回登录页与风控的 selector（§8.1(4)）。
// L2/L3 尚无快照样本，按候选实现并留待第二样本复核（代码处标 TODO(sample2)）。
func defaultLogin() Login {
	return Login{
		Paywall: "div#chargeWallContent",
		Form: []string{ // TODO(sample2): 缺未登录样本，待采集后定稿
			"form#loginForm",
			`input[name*="password"]`,
			".loginBox",
		},
		Captcha: []string{ // TODO(sample2): 缺验证码样本，待采集后定稿
			`iframe[src*="captcha"]`,
			`img[src*="captcha"]`,
			"#captchaImg",
		},
	}
}
