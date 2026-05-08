package email

import "fmt"

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
