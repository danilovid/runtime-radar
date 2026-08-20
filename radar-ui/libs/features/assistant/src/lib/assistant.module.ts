import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { NgModule } from '@angular/core';

import { AssistantDomainModule } from '@cs/domains/assistant';
import { I18nModule } from '@cs/i18n';
import { IntegrationDomainModule } from '@cs/domains/integration';
import { LetDirective } from '@ngrx/component';

import { AssistantFeatureMarkdownPipe } from './pipes/assistant-markdown.pipe';
import { AssistantFeatureMessageComponent } from './components/message/assistant-message.component';
import { AssistantFeatureWidgetComponent } from './components/widget/assistant-widget.component';

/**
 * The chat widget lives in the application shell rather than on a route: it is
 * reachable from every page, and closing it keeps the conversation for as long
 * as the page lives. The chats page is a lazy route of its own, see
 * AssistantFeatureRoutingModule.
 */
@NgModule({
    imports: [AssistantDomainModule, CommonModule, FormsModule, I18nModule, IntegrationDomainModule, LetDirective],
    declarations: [AssistantFeatureMarkdownPipe, AssistantFeatureMessageComponent, AssistantFeatureWidgetComponent],
    exports: [AssistantFeatureWidgetComponent]
})
export class AssistantFeatureModule {}
