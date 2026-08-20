import { ChangeDetectionStrategy, Component, Input } from '@angular/core';

import { AssistantMessage, AssistantRole } from '@cs/domains/assistant';

@Component({
    selector: 'cs-assistant-message',
    templateUrl: './assistant-message.component.html',
    styleUrl: './assistant-message.component.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AssistantFeatureMessageComponent {
    @Input({ required: true }) message!: AssistantMessage;

    readonly assistantRole = AssistantRole;
}
