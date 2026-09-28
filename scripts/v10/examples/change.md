---
format: haft.change/1
id: chg-20260928-35c0ffee
change_key: chg-20260928-35c0ffee
title: Clarify the cancellation state boundary
intent: Make the shipped exclusion and supported pending state explicit while keeping paid cancellation
state: open
created_at: 2026-09-28T12:00:00Z
supersedes: []
tasks:
  - id: correct-code
    text: Correct CanCancel for paid orders and rerun the declared check
    done: false
patches:
  - base: __EXACT_BASE_REF__
    operations:
      - op: MODIFIED
        claim_id: cancellation
        claim:
          id: cancellation
          kind: law
          text: Pending and paid orders may be cancelled; shipped orders may not.
          x-policy:
            review: retain this extension through a claim clarification
          implemented_by:
            - ref: sym:order.go::CanCancel
              covers: Local cancellation decision for the three fixture states
          checks:
            - ref: test:order_test.go::TestCanCancel
              covers: Pending, paid and shipped examples in this fixture only
        reason: Clarify the state boundary after reviewing the declared fixture; the paid-order requirement is unchanged
---
The code correction fixes the failed assertion; the content edit separately clarifies the domain boundary.
