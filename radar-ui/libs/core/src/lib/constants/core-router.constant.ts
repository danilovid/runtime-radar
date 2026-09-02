export enum RouterName {
    DEFAULT = '',
    ADMISSION = 'admission',
    CHATS = 'chats',
    CLUSTERS = 'clusters',
    FORBIDDEN = 'forbidden',
    ERROR = 'error',
    INTEGRATIONS = 'integrations',
    MCP_KEYS = 'mcp-keys',
    RULES = 'rules',
    RUNTIME = 'runtime',
    SETTINGS = 'settings',
    SIGN_IN = 'sign-in',
    SWITCH = 'switch',
    TOKENS = 'tokens',
    USERS = 'users'
}

export enum TranslationDict {
    ADMISSION = 'admission',
    ASSISTANT = 'assistant',
    AUTH = 'auth',
    COMMON = 'common',
    CLUSTER = 'cluster',
    INTEGRATION = 'integration',
    MCP_KEY = 'mcp-key',
    REPORT = 'report',
    RULE = 'rule',
    RUNTIME = 'runtime',
    TOKEN = 'token',
    USER = 'user'
}

export const DEFAULT_ROUTER_NAME = RouterName.RUNTIME;

// The chat widget belongs to the shell, so its dictionary is loaded up front
// rather than per route.
export const DEFAULT_TRANSLATION_DICTS = [TranslationDict.ASSISTANT, TranslationDict.COMMON, TranslationDict.USER];
