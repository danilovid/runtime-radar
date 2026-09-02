import { DateAdapter } from '@koobiq/components/core';
import { DateTime } from 'luxon';
import { Pipe, PipeTransform } from '@angular/core';

import { I18nService } from '@cs/i18n';

@Pipe({
    name: 'mcpKeyExpirationLabel',
    pure: false
})
export class McpKeyFeatureExpirationLabelPipe implements PipeTransform {
    constructor(
        private readonly dateAdapter: DateAdapter<DateTime>,
        private readonly i18nService: I18nService
    ) {}

    transform(dateStr: string, expiresAt?: string | null, invalidatedAt?: string): string {
        if (invalidatedAt) {
            return `${this.i18nService.translate('McpKey.Pseudo.Label.Revoked')} ${dateStr}`;
        }

        const dateTime = this.dateAdapter.parse(expiresAt, undefined);
        const value = dateTime?.diffNow('days').toObject().days || 0;

        return value <= 0
            ? `${this.i18nService.translate('McpKey.Pseudo.Label.Expired')} ${dateStr}`
            : `${this.i18nService.translate('McpKey.Pseudo.Label.Expirating')} ${dateStr}`;
    }
}
