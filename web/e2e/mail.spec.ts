import { expect, test } from '@playwright/test'

// SMTP 未配置路径的端到端(不真发邮件、不碰共享开关,零状态污染)。
test('邮件流(未配置 SMTP):忘记密码恒成功文案/设置页邮件区块/测试发送报未配置', async ({ page }) => {
  // 匿名:登录页 → 忘记密码 → 恒成功
  await page.goto('/login')
  await page.getByRole('link', { name: '忘记密码?' }).click()
  await expect(page).toHaveURL(/\/forgot-password$/)
  await page.getByLabel('邮箱').fill('nobody@img.li')
  await page.getByRole('button', { name: '发送重置邮件' }).click()
  await expect(page.getByText(/若该邮箱已注册/)).toBeVisible()

  // 失效 token 的验证页
  await page.goto('/verify-email?token=bogus')
  await expect(page.getByText('验证失败')).toBeVisible()

  // boss:设置页邮件区块可见,测试发送报未配置
  await page.goto('/login')
  await page.getByLabel('账号').fill('boss')
  await page.getByLabel('密码').fill('bosspass-777')
  await page.getByTestId('auth-submit').click()
  await expect(page.getByTestId('dropzone')).toBeVisible()
  await page.goto('/admin/settings')
  // 设置页已 tab 化,默认停在「基本」,先切到目标区块
  await page.getByRole('button', { name: '邮件 SMTP' }).click()
  await expect(page.getByLabel('SMTP 服务器')).toBeVisible()
  await page.getByLabel('测试收件人').fill('probe@img.li')
  await page.getByRole('button', { name: '发送测试邮件' }).click()
  await expect(page.getByText(/请填写 SMTP 服务器|SMTP 未配置/)).toBeVisible()

  // 资料页未验证徽章
  await page.goto('/settings')
  await expect(page.getByText('未验证')).toBeVisible()
  await page.context().clearCookies()
})
