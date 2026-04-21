import { Pipe, PipeTransform } from '@angular/core';

import { INTEGRATION_AI_PROVIDER_TYPE } from '../constants/integration.constant';
import { IntegrationAIProviderType } from '../interfaces/contract/integration-contract.interface';

@Pipe({
    name: 'integrationAIProviderTypeLocalization',
    pure: false
})
export class IntegrationAIProviderTypeLocalizationPipe implements PipeTransform {
    transform(type?: IntegrationAIProviderType): string {
        const value = INTEGRATION_AI_PROVIDER_TYPE.find((item) => item.id === type);

        return value ? value.localizationKey : '';
    }
}
