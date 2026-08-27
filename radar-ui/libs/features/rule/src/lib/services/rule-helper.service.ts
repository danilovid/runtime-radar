import { Injectable } from '@angular/core';

import { RuleForm } from '@cs/shared';
import { RuleNotifyEntity, RuleType, RuleWhiteList } from '@cs/domains/rule';

@Injectable({
    providedIn: 'root'
})
export class RuleFeatureHelperService {
    static convertFormValuesToNotifyEntity(form: RuleForm): RuleNotifyEntity | null {
        return {
            severity: form.notifySeverity,
            verdict: null,
            targets: form.mailIds
        };
    }

    static convertWhiteListToRequestNode(form: RuleForm): RuleWhiteList {
        const node: RuleWhiteList = {
            threats: [],
            binaries: []
        };

        // Both kinds of rule keep the whitelist in threats. A runtime one lists WASM detectors and,
        // additionally, process binaries; an admission one lists Kyverno policies and has no binaries.
        if (form.type === RuleType.TYPE_ADMISSION) {
            node.threats.push(...form.policies);

            return node;
        }

        node.threats.push(...form.detectors);
        node.binaries.push(...form.binaries);

        return node;
    }
}
