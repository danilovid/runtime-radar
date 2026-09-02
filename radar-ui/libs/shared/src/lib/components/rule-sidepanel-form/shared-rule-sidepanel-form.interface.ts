import { Rule, RuleSeverity, RuleType } from '@cs/domains/rule';

export interface SharedRuleSidepanelFormProps {
    rule: Partial<Rule>;
    isEdit: boolean;
    /**
     * type fixes the kind of rule being authored. When it is not given, the form lets the user pick
     * one, which is what the common rules page needs. A page dedicated to a single kind of rule
     * passes its own type instead and the picker stays hidden.
     */
    type: RuleType;
}

export interface RuleForm {
    name: string;
    type: RuleType;
    namespaces: string[];
    notifySeverity: RuleSeverity;
    mailIds: string[];
    detectors: string[];
    /** policies holds identifiers of Kyverno policies ("<policy name>/<rule name>") for admission rules. */
    policies: string[];
    pods: string[];
    containers: string[];
    nodes: string[];
    binaries: string[];
    imageNames: string[];
    registries: string[];
}
