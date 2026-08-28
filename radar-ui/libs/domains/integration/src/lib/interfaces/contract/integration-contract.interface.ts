interface AbstractIntegration {
    id: string;
    type: IntegrationType;
    name: string;
    skip_check: boolean;
}

export enum IntegrationType {
    EMAIL = 'email',
    SYSLOG = 'syslog',
    WEBHOOK = 'webhook',
    AI = 'ai'
}

export enum IntegrationAIProviderType {
    OPENAI_COMPATIBLE = 'PROVIDER_OPENAI_COMPATIBLE',
    ANTHROPIC = 'PROVIDER_ANTHROPIC',
    OLLAMA = 'PROVIDER_OLLAMA',
    QWEN = 'PROVIDER_QWEN',
    DEEPSEEK = 'PROVIDER_DEEPSEEK',
    GLM = 'PROVIDER_GLM'
}

export enum IntegrationEmailAuthType {
    NONE = 'AUTH_TYPE_NONE',
    CRAM_MD5 = 'AUTH_TYPE_CRAM_MD5',
    LOGIN = 'AUTH_TYPE_LOGIN',
    PLAIN = 'AUTH_TYPE_PLAIN'
}

export interface IntegrationEmailEntity {
    auth_type: IntegrationEmailAuthType;
    from: string;
    server: string;
    username: string;
    password: string;
    ca: string;
    use_tls: boolean;
    use_start_tls: boolean;
    insecure: boolean;
}

export interface IntegrationEmail extends AbstractIntegration {
    type: IntegrationType.EMAIL;
    email: IntegrationEmailEntity;
}

export interface IntegrationSyslogEntity {
    address: string;
}

export interface IntegrationSyslog extends AbstractIntegration {
    type: IntegrationType.SYSLOG;
    syslog: IntegrationSyslogEntity;
}

export interface IntegrationWebhookEntity {
    url: string;
    login: string;
    password: string;
    ca: string;
    insecure: boolean;
}

export interface IntegrationWebhook extends AbstractIntegration {
    type: IntegrationType.WEBHOOK;
    webhook: IntegrationWebhookEntity;
}

/**
 * IntegrationAIScope is one half of the product the built-in assistant may
 * reach. It is a property of the integration rather than of a credential: the
 * assistant answers with the signed-in user's own token, so there is no key to
 * hang a limit on. An empty list offers every tool the user's role allows.
 */
export enum IntegrationAIScope {
    RUNTIME_MONITOR = 'runtime_monitor',
    ADMISSION = 'admission'
}

export interface IntegrationAIEntity {
    provider: IntegrationAIProviderType;
    base_url: string;
    model: string;
    api_key: string;
    is_local: boolean;
    insecure: boolean;
    ca: string;
    scopes?: IntegrationAIScope[];
}

export interface IntegrationAI extends AbstractIntegration {
    type: IntegrationType.AI;
    ai: IntegrationAIEntity;
}

export type Integration = IntegrationEmail | IntegrationSyslog | IntegrationWebhook | IntegrationAI;
