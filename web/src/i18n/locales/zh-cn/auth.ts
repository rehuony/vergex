import type { auth as template } from '@/i18n/locales/en/auth'

export const auth: typeof template = {
  login: '登录',
  username: '用户名',
  email: '邮箱',
  password: '密码',
  loginButton: '登录',
  forgotPassword: '忘记密码？',
  forgotLogin: '忘记账号？',
  resetPasswordTitle: '重置您的密码',
  backToLogin: '返回登录',
  resetPasswordCliIntro:
    '出于安全考虑，密码找回不再通过浏览器进行。请在部署 Vergex 的服务器上运行以下命令：',
  resetPasswordCliSecurityNote:
    '该操作需要服务器的 shell 访问权限，因此即使 Vergex 暴露在公网上，你的账号依然安全',
  resetUserCliIntro:
    '如需清空所有数据并重新开始，请在部署 Vergex 的服务器上运行以下命令：',
  loginFailed: '登录失败，请检查您的邮箱和密码',
  sessionExpired: '登录已过期，请重新登录',
  invalidCredentials: '邮箱或密码错误',
  passwordMismatch: '两次输入的密码不一致',
  exitLogin: '退出登录',
  signIn: '登录',
  loggingIn: '正在登录…',
  resetUserTitle: '重新开始',
  resetUserCliWarning:
    '这会永久删除用户及其全部数据：模型、账户和 API Key、交易员、交易历史和 Telegram 机器人。交易所上的持仓和订单会保留。输入 wipe 确认，然后重启 Vergex',
  loginRequired: {
    title: '需要登录',
    subtitleWithFeature: '登录后才能打开「{featureName}」',
    subtitleDefault: '登录后才能打开这个页面',
    description:
      '你的交易员、账户和模型设置都保存在这台服务器上，登录后才能访问',
    benefit1: '交易员和它们的配置',
    benefit2: '合约交易规划',
    benefit3: '持仓、盈亏和 Agent 决策',
    loginButton: '登录',
    notNow: '暂不登录',
  },
  setup: {
    welcome: '欢迎使用 Vergex',
    subtitle: '创建账号即可开始',
    emailPlaceholder: 'you@example.com',
    passwordPlaceholder: '至少 8 个字符',
    passwordError: '密码至少需要 8 个字符',
    submit: '开始使用',
    submitting: '正在创建账号…',
    failed: '设置失败，请重试',
  },
}
