emptyArr = []
emptyMap = {}
PRINT emptyArr
PRINT emptyMap
PRINT LEN(emptyArr)
PRINT LEN(emptyMap)

data = {"items": [{"name": "pen", "qty": 2}, {"name": "book", "qty": 1}]}
PRINT data.items[0].name
PRINT data["items"][1]["qty"]
data.items[1].qty = 5
PRINT data.items[1].qty
PRINT LEN(data.items)
