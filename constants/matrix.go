package constants

// MatrixSendStatus represents the outcome of sending a Matrix message
type MatrixSendStatus string

const (
	MatrixSendSuccess          MatrixSendStatus = "success"
	MatrixSendFailedEncryption MatrixSendStatus = "failed_encryption"
	MatrixSendFailedSend       MatrixSendStatus = "failed_send"
	MatrixSendFailedForbidden  MatrixSendStatus = "failed_forbidden"
	// MatrixSendTimedOut is a message given up because it couldn't be sent by its deadline
	MatrixSendTimedOut MatrixSendStatus = "timed_out"
	// MatrixSendDropped is a message that stopped being wanted before it was sent
	MatrixSendDropped MatrixSendStatus = "dropped"
)

// AllMatrixSendStatuses contains all valid Matrix send status values
var AllMatrixSendStatuses = []MatrixSendStatus{
	MatrixSendSuccess, MatrixSendFailedEncryption,
	MatrixSendFailedSend, MatrixSendFailedForbidden,
	MatrixSendTimedOut, MatrixSendDropped,
}
