/**
 * Reads the environment into a typed object and validates it at boot. Every
 * route refuses to serve on a missing or malformed value, rather than
 * discovering it at 3am on the first webhook.
 *
 * Port of backend/internal/config/config.go — the validation rules are
 * deliberately identical, including collecting every problem before failing so
 * a misconfigured deploy reports all of its faults in one pass.
 */

export type Env = 'development' | 'staging' | 'production';

export interface Config {
  databaseURL: string;
  env: Env;
  sessionCookieDomain: string;
  footballDataToken: string;
  apiFootballKey: string;
  marzPayAPIUser: string;
  marzPayAPIKey: string;
  marzPayWebhookSecret: string;
  marzPayBaseURL: string;
  publicBaseURL: string;
  modelVersion: string;
  allowedOrigins: string[];
  logLevel: string;
  cronSecret: string;
}

const DEFAULT_MARZPAY_BASE_URL = 'https://wallet.wearemarz.com/api/v1';

function getDefault(key: string, fallback: string): string {
  const value = process.env[key];
  return value !== undefined && value !== '' ? value : fallback;
}

function splitList(value: string): string[] {
  return value
    .split(',')
    .map((part) => part.trim())
    .filter((part) => part !== '');
}

export function isProduction(config: Config): boolean {
  return config.env === 'production';
}

let cached: Config | null = null;

export function loadConfig(): Config {
  if (cached) return cached;

  const problems: string[] = [];

  const databaseURL = process.env.DATABASE_URL ?? '';
  const envRaw = getDefault('ENV', 'development');
  const modelVersion = process.env.MODEL_VERSION ?? '';
  const publicBaseURL = process.env.PUBLIC_BASE_URL ?? '';
  const marzPayBaseURL = getDefault('MARZPAY_BASE_URL', DEFAULT_MARZPAY_BASE_URL);

  if (databaseURL === '') problems.push('DATABASE_URL is required');

  if (envRaw !== 'development' && envRaw !== 'staging' && envRaw !== 'production') {
    problems.push(`ENV "${envRaw}" must be development, staging or production`);
  }
  const env = envRaw as Env;

  if (modelVersion === '') {
    // Stamped onto every prediction, and the accuracy dashboard reports per
    // version. An unstamped prediction cannot be attributed to the model that
    // made it.
    problems.push('MODEL_VERSION is required');
  }

  if (publicBaseURL === '') {
    problems.push('PUBLIC_BASE_URL is required (used to build callback_url)');
  } else {
    let parsed: URL;
    try {
      parsed = new URL(publicBaseURL);
    } catch {
      parsed = null as unknown as URL;
    }
    if (!parsed || !parsed.protocol || !parsed.host) {
      problems.push(`PUBLIC_BASE_URL "${publicBaseURL}" is not an absolute URL`);
    } else if (publicBaseURL.endsWith('/')) {
      problems.push('PUBLIC_BASE_URL must not have a trailing slash');
    }
  }

  try {
    new URL(marzPayBaseURL);
  } catch {
    problems.push(`MARZPAY_BASE_URL "${marzPayBaseURL}" is not a URL`);
  }

  const allowedOrigins = splitList(process.env.ALLOWED_ORIGINS ?? '');
  if (allowedOrigins.length === 0) {
    // CORS is the Next.js origin only. There is no wildcard fallback:
    // defaulting to "*" on a missing value is how a permissive CORS policy
    // reaches production unnoticed.
    problems.push('ALLOWED_ORIGINS is required (comma-separated Next.js origins)');
  }

  const config: Config = {
    databaseURL,
    env,
    sessionCookieDomain: process.env.SESSION_COOKIE_DOMAIN ?? '',
    footballDataToken: process.env.FOOTBALL_DATA_TOKEN ?? '',
    apiFootballKey: process.env.APIFOOTBALL_KEY ?? '',
    marzPayAPIUser: process.env.MARZPAY_API_USER ?? '',
    marzPayAPIKey: process.env.MARZPAY_API_KEY ?? '',
    marzPayWebhookSecret: process.env.MARZPAY_WEBHOOK_SECRET ?? '',
    marzPayBaseURL,
    publicBaseURL,
    modelVersion,
    allowedOrigins,
    logLevel: getDefault('LOG_LEVEL', 'info'),
    cronSecret: process.env.CRON_SECRET ?? '',
  };

  // Secrets are only mandatory in production. Development runs against a fake
  // payment provider and recorded ingestion fixtures, and demanding live
  // credentials to boot locally is what pushes people to paste real ones into
  // a .env they then commit.
  if (env === 'production') {
    const required: Array<[string, string]> = [
      ['FOOTBALL_DATA_TOKEN', config.footballDataToken],
      ['APIFOOTBALL_KEY', config.apiFootballKey],
      ['MARZPAY_API_USER', config.marzPayAPIUser],
      ['MARZPAY_API_KEY', config.marzPayAPIKey],
      ['MARZPAY_WEBHOOK_SECRET', config.marzPayWebhookSecret],
      ['SESSION_COOKIE_DOMAIN', config.sessionCookieDomain],
      ['CRON_SECRET', config.cronSecret],
    ];
    for (const [name, value] of required) {
      if (value === '') problems.push(`${name} is required in production`);
    }
  }

  if (problems.length > 0) {
    throw new Error(`invalid configuration:\n  - ${problems.join('\n  - ')}`);
  }

  cached = config;
  return config;
}

/** Reports whether the live payment client can be built. */
export function marzPayConfigured(config: Config): boolean {
  return config.marzPayAPIUser !== '' && config.marzPayAPIKey !== '';
}

export function webhookCallbackURL(config: Config): string {
  return `${config.publicBaseURL}/v1/webhooks/marzpay`;
}
