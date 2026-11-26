IMPORT { CalcTax, CalcTotal, Greet, LABEL } FROM "./mod_helper.bas"

Greet("ADA")
PRINT LABEL & ": " & STR(CalcTotal("100.00"))
PRINT CalcTax("50.00")
