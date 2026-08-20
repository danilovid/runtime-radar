import { CommonModule } from '@angular/common';
import { NgModule } from '@angular/core';

import { AssistantDomainModule } from '@cs/domains/assistant';
import { I18nModule } from '@cs/i18n';

import { AssistantFeatureChatsContainer } from './containers/chats/assistant-chats.container';
import { AssistantFeatureRoutingModule } from './assistant-routing.module';

/** The "AI chat" section: a page listing the conversations of this session. */
@NgModule({
    imports: [AssistantDomainModule, AssistantFeatureRoutingModule, CommonModule, I18nModule],
    declarations: [AssistantFeatureChatsContainer]
})
export class AssistantPageFeatureModule {}
