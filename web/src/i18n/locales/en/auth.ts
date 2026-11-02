// Signing in, first setup and the checks around them.
export const auth = {
  login: 'Sign In',
  username: 'Username',
  email: 'Email',
  password: 'Password',
  loginButton: 'Sign In',
  forgotPassword: 'Forgot password?',
  forgotLogin: 'Forgot login?',
  resetPasswordTitle: 'Reset your password',
  backToLogin: 'Back to Login',
  resetPasswordCliIntro:
    'For security, password recovery is no longer available from the browser. Run this command on the server where Vergex is installed:',
  resetPasswordCliSecurityNote:
    'This requires shell access to the server, which keeps your login safe even when Vergex is exposed to the internet',
  resetUserCliIntro:
    'To wipe everything and start over, run this command on the server where Vergex is installed:',
  loginFailed: 'Login failed. Please check your email and password',
  sessionExpired: 'Session expired, please login again',
  invalidCredentials: 'Invalid email or password',
  passwordMismatch: 'Passwords do not match',
  exitLogin: 'Sign Out',
  signIn: 'Sign In',
  loggingIn: 'Signing in…',
  resetUserTitle: 'Start over',
  resetUserCliWarning:
    'This permanently deletes the user and all their data: models, accounts and API keys, traders, trade history and the Telegram bot. Positions and orders on the exchange stay open. Type wipe to confirm, then restart Vergex',
  loginRequired: {
    title: 'Sign in required',
    subtitleWithFeature: 'Sign in to open {featureName}',
    subtitleDefault: 'Sign in to open this page',
    description:
      'Your traders, accounts and model settings are kept on this server and are only available after you sign in',
    benefit1: 'Traders and their settings',
    benefit2: 'Futures trade planning',
    benefit3: 'Positions, P&L and agent decisions',
    loginButton: 'Sign in',
    notNow: 'Not now',
  },
  setup: {
    welcome: 'Welcome to Vergex',
    subtitle: 'Create your login to get started',
    emailPlaceholder: 'you@example.com',
    passwordPlaceholder: 'At least 8 characters',
    passwordError: 'Password must be at least 8 characters',
    submit: 'Get started',
    submitting: 'Creating login…',
    failed: 'Setup failed, please try again',
  },
}
