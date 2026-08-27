import { KBQ_SIDEPANEL_DATA } from '@koobiq/components/sidepanel';
import { KbqCodeBlockFile } from '@koobiq/components/code-block';
import { ChangeDetectionStrategy, Component, Inject, OnInit } from '@angular/core';

import { AdmissionSidepanelPolicyProps } from '../../interfaces/admission-sidepanel.interface';

@Component({
    templateUrl: './admission-sidepanel-policy.component.html',
    styleUrl: './admission-sidepanel-policy.component.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AdmissionFeatureSidepanelPolicyComponent implements OnInit {
    files: KbqCodeBlockFile[] = [];

    constructor(@Inject(KBQ_SIDEPANEL_DATA) public readonly props: Partial<AdmissionSidepanelPolicyProps>) {}

    ngOnInit() {
        if (this.props.yaml) {
            this.files.push({
                filename: 'policy.yaml',
                content: this.props.yaml,
                language: 'yaml'
            });
        }
    }
}
