import { AfterViewInit, ChangeDetectionStrategy, Component, Inject } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { KBQ_SIDEPANEL_DATA, KbqSidepanelRef } from '@koobiq/components/sidepanel';
import { Observable, debounceTime, distinctUntilChanged, map, startWith } from 'rxjs';

import { FormScheme, CoreUtilsService as utils } from '@cs/core';

import { AdmissionFeatureYamlValidator } from '../../validators/admission-yaml.validator';
import { AdmissionSidepanelPolicyFormProps } from '../../interfaces/admission-sidepanel.interface';

/**
 * The manifest of a source is edited as is. Its spec.validationActions is overwritten by
 * admission-monitor from the source's action, so it is intentionally absent from the default.
 */
const DEFAULT_ADMISSION_POLICY_YAML = `apiVersion: policies.kyverno.io/v1
kind: ValidatingPolicy
metadata:
  name: "policy-name"`;

interface AdmissionPolicyFormValues {
    name: string;
    description: string;
    yaml: string;
}

@Component({
    templateUrl: './admission-sidepanel-policy-form.component.html',
    styleUrl: './admission-sidepanel-policy-form.component.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AdmissionFeatureSidepanelPolicyFormComponent implements AfterViewInit {
    readonly form: FormGroup<FormScheme<AdmissionPolicyFormValues>> = this.formBuilder.group({
        name: ['', Validators.required],
        description: ['', Validators.required],
        yaml: [DEFAULT_ADMISSION_POLICY_YAML, [Validators.required, AdmissionFeatureYamlValidator.isYamlCodeValid()]]
    });

    readonly isFormValid$: Observable<boolean> = this.form.valueChanges.pipe(
        startWith(this.form.value),
        /* eslint @typescript-eslint/no-magic-numbers: "off" */
        debounceTime(250),
        distinctUntilChanged(),
        map(() => utils.isFormValid(this.form.controls))
    );

    constructor(
        private readonly formBuilder: FormBuilder,
        private readonly sidepanelRef: KbqSidepanelRef,
        @Inject(KBQ_SIDEPANEL_DATA) public readonly props: Partial<AdmissionSidepanelPolicyFormProps>
    ) {}

    ngAfterViewInit() {
        if (this.props.isEdit) {
            this.form.patchValue({
                name: this.props.name,
                description: this.props.description,
                yaml: this.props.yaml
            });
        }
    }

    confirm() {
        if (this.form.valid) {
            const formValues = utils.getFormValues<AdmissionPolicyFormValues>(this.form.controls);
            this.sidepanelRef.close(utils.getTrimmedFormValues<AdmissionPolicyFormValues>(formValues));
        }
    }

    cancel() {
        this.sidepanelRef.close(undefined);
    }
}
