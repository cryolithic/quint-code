---
format: haft/2
kind: spec
title: Alpha cancellation rule
about: domain:Alpha.OrderCancellation
slug: alpha-cancel
receiving_use: Check the local cancellation implementation
x-fixture:
  preserved: true
  source: packaged-alpha-example
claims:
  - id: cancellation
    kind: law
    text: A paid order may be cancelled.
    x-policy:
      review: retain this extension through a claim clarification
    implemented_by:
      - ref: sym:order.go::CanCancel
        covers: Local cancellation decision for the three fixture states
    checks:
      - ref: test:order_test.go::TestCanCancel
        covers: Pending, paid and shipped examples in this fixture only
---
This is current authored content. It does not bind a decision or certify a check.
