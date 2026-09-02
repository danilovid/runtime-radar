import { CommonModule } from '@angular/common';
import { NgModule } from '@angular/core';

import { AssistantDomainModule } from '@cs/domains/assistant';
import { I18nModule } from '@cs/i18n';
import { IntegrationDomainModule } from '@cs/domains/integration';
import { SharedModule } from '@cs/shared';

import { AssistantFeatureChatsContainer } from './containers/chats/assistant-chats.container';
import { AssistantFeatureRoutingModule } from './assistant-routing.module';

/** The "AI chat" section: a page listing the conversations of this session. */
@NgModule({
    imports: [
        AssistantDomainModule,
        AssistantFeatureRoutingModule,
        CommonModule,
        I18nModule,
        IntegrationDomainModule,
        SharedModule
    ],
    declarations: [AssistantFeatureChatsContainer]
})
export class AssistantPageFeatureModule {}
