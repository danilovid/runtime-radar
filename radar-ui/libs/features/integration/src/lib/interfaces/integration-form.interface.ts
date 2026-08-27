import { IntegrationAIProviderType, IntegrationEmailAuthType } from '@cs/domains/integration';

export enum IntegrationProtocolType {
    NONE = 'NONE',
    TLS = 'TLS',
    START_TLS = 'START_TLS'
}

export interface IntegrationEmailForm {
    name: string;
    server: string;
    authType: IntegrationEmailAuthType;
    username: string;
    password: string;
    ca: string;
    from: string;
    protocol: IntegrationProtocolType;
    isInsecure: boolean;
}

export interface IntegrationSyslogForm {
    name: string;
    address: string;
}

export interface IntegrationWebhookForm {
    name: string;
    url: string;
    login: string;
    password: string;
    ca: string;
    isInsecure: boolean;
}

export interface IntegrationAIForm {
    name: string;
    provider: IntegrationAIProviderType;
    baseUrl: string;
    model: string;
    apiKey: string;
    ca: string;
    isLocal: boolean;
    isInsecure: boolean;
}
