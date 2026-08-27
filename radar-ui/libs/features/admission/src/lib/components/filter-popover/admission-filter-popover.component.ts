import { KbqTagInputEvent } from '@koobiq/components/tags';
import { PopUpSizes } from '@koobiq/components/core';
import { ChangeDetectionStrategy, Component, EventEmitter, Input, OnChanges, Output } from '@angular/core';
import { FormArray, FormBuilder, FormGroup, Validators } from '@angular/forms';

import { FORM_SEPARATOR_KEY_CODES, FormScheme, CoreUtilsService as utils } from '@cs/core';

import { ADMISSION_FILTER_INITIAL_STATE } from '../../constants/admission-filter.constant';
import { AdmissionEventFilters } from '../../interfaces/admission-events.interface';

@Component({
    selector: 'cs-admission-feature-filter-popover-component',
    templateUrl: './admission-filter-popover.component.html',
    styleUrl: './admission-filter-popover.component.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AdmissionFeatureFilterPopoverComponent implements OnChanges {
    @Input() filters?: AdmissionEventFilters | null;

    @Output() filterChange = new EventEmitter<AdmissionEventFilters>();

    readonly form: FormGroup<
        FormScheme<
            AdmissionEventFilters,
            never,
            'resourceKind' | 'resourceNamespace' | 'resourceName' | 'nodeName' | 'imageNames'
        >
    > = this.formBuilder.group({
        resourceKind: this.formBuilder.array<string>([]),
        resourceNamespace: this.formBuilder.array<string>([]),
        resourceName: this.formBuilder.array<string>([]),
        nodeName: this.formBuilder.array<string>([]),
        imageNames: this.formBuilder.array<string>([]),
        blocked: [false],
        hasIncident: [false]
    });

    readonly separatorKeyCodes = FORM_SEPARATOR_KEY_CODES;

    readonly popoverSizes = PopUpSizes;

    get resourceKindControl(): FormArray {
        return this.form.get('resourceKind') as FormArray;
    }

    get resourceNamespaceControl(): FormArray {
        return this.form.get('resourceNamespace') as FormArray;
    }

    get resourceNameControl(): FormArray {
        return this.form.get('resourceName') as FormArray;
    }

    get nodeNameControl(): FormArray {
        return this.form.get('nodeName') as FormArray;
    }

    get imageNamesControl(): FormArray {
        return this.form.get('imageNames') as FormArray;
    }

    constructor(private readonly formBuilder: FormBuilder) {}

    ngOnChanges() {
        if (!this.filters) {
            return;
        }

        [
            this.resourceKindControl,
            this.resourceNamespaceControl,
            this.resourceNameControl,
            this.nodeNameControl,
            this.imageNamesControl
        ].forEach((control) => control.clear());

        utils.setArrayControlValue(this.resourceKindControl, this.filters.resourceKind, this.formBuilder);
        utils.setArrayControlValue(this.resourceNamespaceControl, this.filters.resourceNamespace, this.formBuilder);
        utils.setArrayControlValue(this.resourceNameControl, this.filters.resourceName, this.formBuilder);
        utils.setArrayControlValue(this.nodeNameControl, this.filters.nodeName, this.formBuilder);
        utils.setArrayControlValue(this.imageNamesControl, this.filters.imageNames, this.formBuilder);

        this.form.patchValue({
            blocked: this.filters.blocked,
            hasIncident: this.filters.hasIncident
        });
    }

    addEntity(event: KbqTagInputEvent, control: FormArray) {
        const value = event.value.trim();

        if (value) {
            control.push(this.formBuilder.control(value, Validators.required));
            event.input.value = '';
        }
    }

    removeEntity(control: FormArray, id: number) {
        control.removeAt(id);
    }

    apply() {
        this.filterChange.emit(utils.getFormValues<AdmissionEventFilters>(this.form.controls));
    }

    reset() {
        this.filterChange.emit({ ...ADMISSION_FILTER_INITIAL_STATE });
    }
}
