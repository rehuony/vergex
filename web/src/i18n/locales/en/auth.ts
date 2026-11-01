// Signing in, registering, first setup and the checks around them.
export const auth = {
  login: 'Sign In',
  register: 'Sign Up',
  username: 'Username',
  email: 'Email',
  password: 'Password',
  confirmPassword: 'Confirm Password',
  passwordRequirements: 'Password requirements',
  passwordRuleMinLength: 'Minimum 8 characters',
  passwordRuleUppercase: 'At least 1 uppercase letter',
  passwordRuleLowercase: 'At least 1 lowercase letter',
  passwordRuleNumber: 'At least 1 number',
  passwordRuleSpecial: 'At least 1 special character (@#$%!&*?)',
  passwordRuleMatch: 'Passwords match',
  passwordNotMeetRequirements:
    'Password does not meet the security requirements',
  registerTitle: 'Create a new account',
  loginButton: 'Sign In',
  registerButton: 'Sign Up',
  hasAccount: 'Already have an account?',
  forgotPassword: 'Forgot password?',
  forgotAccount: 'Forgot account?',
  resetPasswordTitle: 'Reset your password',
  backToLogin: 'Back to Login',
  resetPasswordCliIntro:
    'For security, password recovery is no longer available from the browser. Run this command on the server where Vergex is installed:',
  resetPasswordCliSecurityNote:
    'This requires shell access to the server, which keeps your account safe even when Vergex is exposed to the internet.',
  resetAccountCliIntro:
    'To wipe everything and start over, run this command on the server where Vergex is installed:',
  loginFailed: 'Login failed. Please check your email and password.',
  registrationFailed: 'Registration failed. Please try again.',
  sessionExpired: 'Session expired, please login again',
  invalidCredentials: 'Invalid email or password',
  passwordMismatch: 'Passwords do not match',
  exitLogin: 'Sign Out',
  signIn: 'Sign In',
  loggingIn: 'Signing in…',
  authCoverBadge: 'Your trading workspace',
  authCoverTitleLead: 'Agent-powered',
  authCoverTitleAccent: 'Trading Terminal',
  authCoverBody:
    'Connect your own models and exchange accounts, describe strategies in plain language, and let an agent read the market and trade around the clock.',
  authStatExchanges: 'Exchanges',
  authStatModels: 'Model providers',
  authStatAlwaysOn: 'Always on',
  resetAccountTitle: 'Reset your account',
  resetAccountCliWarning:
    'This permanently deletes the account and all its data: models, exchange accounts and API keys, traders, strategies, trade history and the Telegram bot. Positions and orders on the exchange stay open. Type wipe to confirm, then restart Vergex.',
  registerIntro:
    'Set up your Vergex account. You can connect models and exchange accounts after you sign in.',
  registering: 'Creating account…',
  betaCodeLabel: 'Access code',
  betaCodeRequired: 'Enter your six-character access code',
  passwordMin8: 'Password must be at least 8 characters',
  registrationClosed: 'Account already set up',
  registrationClosedMessage:
    'Vergex runs with a single account, and this instance already has one. Sign in with it instead.',
  environmentCheck: {
    checking: 'Checking...',
    description:
      'Automatically verifying whether this browser context allows Web Crypto before entering sensitive keys.',
    secureTitle: 'Secure context detected',
    secureDesc:
      'Web Crypto API is available. You can continue entering secrets with encryption enabled.',
    insecureTitle: 'Insecure context detected',
    insecureDesc:
      'This page is not running over HTTPS or a trusted localhost origin, so browsers block Web Crypto calls.',
    tipsTitle: 'How to fix:',
    tipHTTPS:
      'Serve the dashboard over HTTPS with a valid certificate (IP origins also need TLS).',
    tipLocalhost:
      'During development, open the app via http://localhost or 127.0.0.1.',
    tipIframe:
      'Avoid embedding the app in insecure HTTP iframes or reverse proxies that strip HTTPS.',
    unsupportedTitle: 'Browser does not expose Web Crypto',
    unsupportedDesc:
      'Open Vergex over HTTPS (or http://localhost during development) and avoid insecure iframes/reverse proxies so the browser can enable Web Crypto.',
    summary: 'Current origin: {origin} • Protocol: {protocol}',
    disabledTitle: 'Transport encryption disabled',
    disabledDesc:
      'Server-side transport encryption is disabled. API keys will be transmitted in plaintext. Enable TRANSPORT_ENCRYPTION=true for enhanced security.',
  },
  loginRequired: {
    title: 'Sign in required',
    subtitleWithFeature: 'Sign in to open {featureName}.',
    subtitleDefault: 'Sign in to open this page.',
    description:
      'Your traders, exchange connections and model settings are kept on this server and are only available after you sign in.',
    benefit1: 'Traders and strategies',
    benefit2: 'Frontier trade planning',
    benefit3: 'Positions, P&L and agent decisions',
    loginButton: 'Sign in',
    notNow: 'Not now',
  },
  localWorkspace: 'Local workspace',
  setup: {
    welcome: 'Welcome to Vergex',
    subtitle: 'Create your account to get started',
    emailPlaceholder: 'you@example.com',
    passwordPlaceholder: 'At least 8 characters',
    passwordError: 'Password must be at least 8 characters',
    submit: 'Get started',
    submitting: 'Creating account…',
    failed: 'Setup failed, please try again',
  },
}
