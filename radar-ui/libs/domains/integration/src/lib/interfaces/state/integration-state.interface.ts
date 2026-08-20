import { EntityState } from '@ngrx/entity';

import { LoadStatus } from '@cs/core';

import {
    IntegrationAI,
    IntegrationEmail,
    IntegrationSyslog,
    IntegrationType,
    IntegrationWebhook
} from '../contract/integration-contract.interface';

export type IntegrationAIEntityState = EntityState<IntegrationAI>;

export type IntegrationEmailEntityState = EntityState<IntegrationEmail>;

export type IntegrationSyslogEntityState = EntityState<IntegrationSyslog>;

export type IntegrationWebhookEntityState = EntityState<IntegrationWebhook>;

export interface IntegrationState {
    loadStatus: LoadStatus;
    loadedTypes: IntegrationType[];
    lastUpdate: number;
    ai: IntegrationAIEntityState;
    email: IntegrationEmailEntityState;
    syslog: IntegrationSyslogEntityState;
    webhook: IntegrationWebhookEntityState;
}
