package constants

// MatrixSendStatus represents the outcome of sending a Matrix message
type MatrixSendStatus string

const (
	MatrixSendSuccess          MatrixSendStatus = "success"
	MatrixSendFailedEncryption MatrixSendStatus = "failed_encryption"
	MatrixSendFailedSend       MatrixSendStatus = "failed_send"
	MatrixSendFailedForbidden  MatrixSendStatus = "failed_forbidden"
)

// AllMatrixSendStatuses contains all valid Matrix send status values
var AllMatrixSendStatuses = []MatrixSendStatus{
	MatrixSendSuccess, MatrixSendFailedEncryption,
	MatrixSendFailedSend, MatrixSendFailedForbidden,
}
