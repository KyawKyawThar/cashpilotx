package email

import "fmt"

func digestEmailBody(d DigestData) string {
	overdueSection := ""
	if d.OverdueCount > 0 {
		overdueSection = fmt.Sprintf(`
        <tr><td style="padding:12px 0;border-bottom:1px solid #f0f0f0;">
          <table width="100%%" cellpadding="0" cellspacing="0"><tr>
            <td style="color:#e05252;font-weight:700;font-size:14px;">⚠ Overdue</td>
            <td align="right" style="color:#e05252;font-weight:700;font-size:14px;">%d invoice%s · %s %s</td>
          </tr></table>
        </td></tr>`, d.OverdueCount, pluralS(d.OverdueCount), d.Currency, formatAmount(d.OverdueTotal))
	}
	dueTodaySection := ""
	if d.DueTodayCount > 0 {
		dueTodaySection = fmt.Sprintf(`
        <tr><td style="padding:12px 0;">
          <table width="100%%" cellpadding="0" cellspacing="0"><tr>
            <td style="color:#c9a646;font-weight:700;font-size:14px;">📅 Due Today</td>
            <td align="right" style="color:#c9a646;font-weight:700;font-size:14px;">%d invoice%s · %s %s</td>
          </tr></table>
        </td></tr>`, d.DueTodayCount, pluralS(d.DueTodayCount), d.Currency, formatAmount(d.DueTodayTotal))
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family:sans-serif;background:#f4f4f4;margin:0;padding:0;">
  <table width="100%%" cellpadding="0" cellspacing="0">
    <tr><td align="center" style="padding:40px 0;">
      <table width="520" cellpadding="0" cellspacing="0" style="background:#fff;border-radius:10px;overflow:hidden;">
        <tr><td style="background:#070B14;padding:28px 36px;">
          <span style="color:#10B981;font-size:20px;font-weight:800;letter-spacing:-0.03em;">CashPilotX</span>
          <span style="color:#526176;font-size:12px;font-family:monospace;margin-left:10px;">Daily Summary</span>
        </td></tr>
        <tr><td style="padding:28px 36px;">
          <p style="color:#1a1a1a;font-size:16px;font-weight:700;margin:0 0 6px;">Good morning, %s.</p>
          <p style="color:#888;font-size:13px;margin:0 0 24px;">Here's your cash recovery summary for today.</p>
          <table width="100%%" cellpadding="0" cellspacing="0" style="border:1px solid #f0f0f0;border-radius:8px;padding:4px 16px;">
            %s%s
          </table>
          <div style="margin-top:28px;text-align:center;">
            <a href="%s" style="background:#10B981;color:#fff;text-decoration:none;padding:12px 32px;border-radius:8px;font-weight:700;font-size:14px;display:inline-block;">Open CashPilotX →</a>
          </div>
        </td></tr>
        <tr><td style="background:#f9f9f9;padding:16px 36px;text-align:center;">
          <p style="color:#aaa;font-size:11px;margin:0;">You receive this because you have a CashPilotX account. <a href="%s/settings" style="color:#aaa;">Manage notifications</a></p>
        </td></tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, d.Name, overdueSection, dueTodaySection, d.AppURL, d.AppURL)
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func formatAmount(minor int64) string {
	return fmt.Sprintf("%d", minor/100)
}

func otpEmailBody(name, otp string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family:sans-serif;background:#f4f4f4;margin:0;padding:0;">
  <table width="100%%" cellpadding="0" cellspacing="0">
    <tr><td align="center" style="padding:40px 0;">
      <table width="480" cellpadding="0" cellspacing="0" style="background:#fff;border-radius:8px;padding:40px;">
        <tr><td>
          <h2 style="color:#1a1a1a;margin:0 0 8px;">PIN Reset Code</h2>
          <p style="color:#555;margin:0 0 24px;">Hi %s, use the code below to reset your CashPilotX PIN.</p>
          <div style="background:#f0f4ff;border-radius:8px;padding:24px;text-align:center;letter-spacing:12px;font-size:36px;font-weight:700;color:#3b5bdb;">%s</div>
          <p style="color:#888;font-size:13px;margin:24px 0 0;">This code expires in <strong>10 minutes</strong>. If you did not request a PIN reset, ignore this email.</p>
        </td></tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, name, otp)
}

func registrationOTPEmailBody(otp string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family:sans-serif;background:#f4f4f4;margin:0;padding:0;">
  <table width="100%%" cellpadding="0" cellspacing="0">
    <tr><td align="center" style="padding:40px 0;">
      <table width="480" cellpadding="0" cellspacing="0" style="background:#fff;border-radius:8px;padding:40px;">
        <tr><td>
          <h2 style="color:#1a1a1a;margin:0 0 8px;">Verify Your Email</h2>
          <p style="color:#555;margin:0 0 24px;">Welcome to CashPilotX! Use the code below to complete your registration.</p>
          <div style="background:#f0f4ff;border-radius:8px;padding:24px;text-align:center;letter-spacing:12px;font-size:36px;font-weight:700;color:#3b5bdb;">%s</div>
          <p style="color:#888;font-size:13px;margin:24px 0 0;">This code expires in <strong>10 minutes</strong>. If you did not create an account, ignore this email.</p>
        </td></tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, otp)
}
