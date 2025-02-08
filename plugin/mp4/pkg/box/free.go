package box

type FreeBox = DataBox

func CreateFreeBox(data []byte) *FreeBox {
	return CreateDataBox(TypeFREE, data)
}

func init() {
	RegisterBox[*FreeBox](TypeFREE)
}
